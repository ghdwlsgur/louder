package analyzer

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
	"github.com/ghdwlsgur/louder/internal/notifier"
	"github.com/ghdwlsgur/louder/internal/storage"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrBudgetPolicyNotFound           = errors.New("budget policy not found")
	ErrWebhookSecretKeyMissing        = errors.New("Teams webhook Secret is missing TEAMS_WEBHOOK_URL")
	ErrBudgetNotificationStatusUpdate = errors.New("budget notification status could not be updated")
)

type TeamsNotifierFactory func(string) (notifier.Notifier, error)

func RunBudgetPolicy(ctx context.Context, kube client.Client, reader storage.CostReader, namespace, policyName string, newTeamsNotifier TeamsNotifierFactory, now time.Time) ([]BudgetThresholdIntent, error) {
	var budget v1alpha1.BudgetPolicy
	if err := kube.Get(ctx, types.NamespacedName{Namespace: namespace, Name: policyName}, &budget); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrBudgetPolicyNotFound
		}
		return nil, err
	}
	var accounts v1alpha1.CloudAccountList
	if err := kube.List(ctx, &accounts, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	analysisAccounts := accounts.Items
	var runErrors []error
	if runReader, ok := reader.(storage.CollectionRunReader); ok {
		scopes := make([]storage.AccountScope, 0, len(accounts.Items))
		for _, account := range accounts.Items {
			if account.Spec.AccountID != "" && matchesSelector(account.Spec.Metadata, budget.Spec.Selector) {
				scopes = append(scopes, storage.AccountScope{Provider: account.Spec.Provider, BillingAccountID: account.Spec.AccountID})
			}
		}
		if len(scopes) > 0 {
			utcNow := now.UTC()
			from := time.Date(utcNow.Year(), utcNow.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -7)
			runs, readErr := runReader.ReadCollectionRuns(ctx, scopes, from, utcNow)
			if readErr != nil {
				runErrors = append(runErrors, fmt.Errorf("read collection freshness metadata: %w", readErr))
			} else {
				analysisAccounts = excludeStaleAccounts(accounts.Items, runs, now)
				for i := range accounts.Items {
					account := &accounts.Items[i]
					if !matchesSelector(account.Spec.Metadata, budget.Spec.Selector) || account.Spec.AccountID == "" {
						continue
					}
					assessment := assessDataFreshness(account.Spec.Provider, runsForAccount(runs, account.Spec.Provider, account.Spec.AccountID), now)
					updateCloudAccountFreshnessStatus(ctx, kube, account, assessment, now, &runErrors)
				}
			}
		}
	}
	var policies v1alpha1.NotificationPolicyList
	if err := kube.List(ctx, &policies, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	intents, err := EvaluateStoredBudget(ctx, reader, budget, analysisAccounts, now)
	if err != nil {
		return nil, err
	}
	month := now.UTC().Format("2006-01")
	intents = pendingBudgetThresholds(intents, budget.Status, month)
	resolve := func(ctx context.Context, policy v1alpha1.NotificationPolicy) (notifier.Notifier, error) {
		if newTeamsNotifier == nil {
			return nil, ErrNotifierRequired
		}
		var secret corev1.Secret
		if err := kube.Get(ctx, types.NamespacedName{Namespace: policy.Namespace, Name: policy.Spec.CredentialRef.Name}, &secret); err != nil {
			return nil, err
		}
		endpoint := secret.Data["TEAMS_WEBHOOK_URL"]
		if len(endpoint) == 0 {
			return nil, ErrWebhookSecretKeyMissing
		}
		return newTeamsNotifier(string(endpoint))
	}
	if len(intents) > 0 {
		if err := NotifyBudgetIntents(ctx, budget, intents, analysisAccounts, policies.Items, resolve); err != nil {
			runErrors = append(runErrors, err)
		} else {
			notified := append([]int32(nil), intentsToPercentages(intents)...)
			if budget.Status.LastNotifiedMonth == month {
				notified = append(notified, budget.Status.NotifiedThresholds...)
			}
			sort.Slice(notified, func(i, j int) bool { return notified[i] < notified[j] })
			budget.Status.LastNotifiedMonth = month
			budget.Status.NotifiedThresholds = compactThresholds(notified)
			if err := kube.Status().Update(ctx, &budget); err != nil {
				runErrors = append(runErrors, ErrBudgetNotificationStatusUpdate)
			}
		}
	}
	forecastIntent, err := EvaluateStoredBudgetForecast(ctx, reader, budget, analysisAccounts, now)
	if err != nil {
		runErrors = append(runErrors, err)
	} else if forecastIntent != nil && budget.Status.LastNotifiedForecastMonth != month {
		if err := NotifyBudgetForecastIntent(ctx, budget, *forecastIntent, analysisAccounts, policies.Items, resolve); err != nil {
			runErrors = append(runErrors, err)
		} else {
			budget.Status.LastNotifiedForecastMonth = month
			if err := kube.Status().Update(ctx, &budget); err != nil {
				runErrors = append(runErrors, ErrBudgetNotificationStatusUpdate)
			}
		}
	}
	anomalyIntents, err := EvaluateStoredDailyCostAnomalies(ctx, reader, budget, analysisAccounts, now)
	if err != nil {
		runErrors = append(runErrors, err)
	} else if len(anomalyIntents) > 0 {
		receipts, notifyErr := NotifyDailyAnomalyIntentsWithReceipts(ctx, budget, anomalyIntents, analysisAccounts, policies.Items, resolve, budget.Status.NotifiedDailyAnomalies)
		if len(receipts) > 0 {
			anomalyDate := anomalyIntents[0].Date.UTC().Format("2006-01-02")
			budget.Status.NotifiedDailyAnomalies = mergeDailyAnomalyReceipts(budget.Status.NotifiedDailyAnomalies, receipts, anomalyDate)
			budget.Status.LastNotifiedAnomalyDate = anomalyDate
			if err := kube.Status().Update(ctx, &budget); err != nil {
				runErrors = append(runErrors, ErrBudgetNotificationStatusUpdate)
			}
		}
		if notifyErr != nil {
			runErrors = append(runErrors, notifyErr)
		}
	}
	return intents, errors.Join(runErrors...)
}

func mergeDailyAnomalyReceipts(existing, delivered []v1alpha1.DailyAnomalyNotificationReceipt, date string) []v1alpha1.DailyAnomalyNotificationReceipt {
	merged := make([]v1alpha1.DailyAnomalyNotificationReceipt, 0, len(existing)+len(delivered))
	for _, receipt := range existing {
		if receipt.Date == date && !hasDailyAnomalyReceipt(merged, receipt) {
			merged = append(merged, receipt)
		}
	}
	for _, receipt := range delivered {
		if receipt.Date == date && !hasDailyAnomalyReceipt(merged, receipt) {
			merged = append(merged, receipt)
		}
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Provider != merged[j].Provider {
			return merged[i].Provider < merged[j].Provider
		}
		if merged[i].BillingAccountID != merged[j].BillingAccountID {
			return merged[i].BillingAccountID < merged[j].BillingAccountID
		}
		return merged[i].NotificationPolicyName < merged[j].NotificationPolicyName
	})
	return merged
}

func pendingBudgetThresholds(intents []BudgetThresholdIntent, status v1alpha1.BudgetPolicyStatus, month string) []BudgetThresholdIntent {
	if status.LastNotifiedMonth != month {
		return intents
	}
	delivered := make(map[int32]struct{}, len(status.NotifiedThresholds))
	for _, threshold := range status.NotifiedThresholds {
		delivered[threshold] = struct{}{}
	}
	pending := make([]BudgetThresholdIntent, 0, len(intents))
	for _, intent := range intents {
		if _, exists := delivered[intent.ThresholdPercent]; !exists {
			pending = append(pending, intent)
		}
	}
	return pending
}

func intentsToPercentages(intents []BudgetThresholdIntent) []int32 {
	thresholds := make([]int32, len(intents))
	for i, intent := range intents {
		thresholds[i] = intent.ThresholdPercent
	}
	return thresholds
}

func compactThresholds(sorted []int32) []int32 {
	if len(sorted) == 0 {
		return sorted
	}
	compacted := sorted[:1]
	for _, threshold := range sorted[1:] {
		if threshold != compacted[len(compacted)-1] {
			compacted = append(compacted, threshold)
		}
	}
	return compacted
}

func runsForAccount(runs []storage.CollectionRun, providerName, accountID string) []storage.CollectionRun {
	matched := make([]storage.CollectionRun, 0)
	for _, run := range runs {
		if run.Provider == providerName && run.BillingAccountID == accountID {
			matched = append(matched, run)
		}
	}
	return matched
}

func updateCloudAccountFreshnessStatus(ctx context.Context, kube client.Client, account *v1alpha1.CloudAccount, assessment dataFreshnessAssessment, now time.Time, errs *[]error) {
	previous := account.DeepCopy().Status
	if assessment.run != nil {
		run := assessment.run
		account.Status.LastSuccessfulCollectionWindow = &v1alpha1.CollectionWindow{Start: metav1.NewTime(run.WindowStart), End: metav1.NewTime(run.WindowEnd)}
		if !run.LatestUsageEnd.IsZero() {
			observed := metav1.NewTime(run.LatestUsageEnd)
			account.Status.LastObservedUsagePeriodEnd = &observed
		}
		if !run.DataIngestedAt.IsZero() {
			ingested := metav1.NewTime(run.DataIngestedAt)
			account.Status.LastCostDataIngestedAt = &ingested
		}
	}
	apiMeta.SetStatusCondition(&account.Status.Conditions, metav1.Condition{
		Type: "DataFresh", Status: freshnessConditionStatus(assessment.state), Reason: assessment.reason,
		Message: assessment.reason, ObservedGeneration: account.Generation, LastTransitionTime: metav1.NewTime(now.UTC()),
	})
	if reflect.DeepEqual(previous, account.Status) {
		return
	}
	if err := kube.Status().Update(ctx, account); err != nil {
		*errs = append(*errs, fmt.Errorf("update CloudAccount %s/%s freshness status: %w", account.Namespace, account.Name, err))
	}
}
