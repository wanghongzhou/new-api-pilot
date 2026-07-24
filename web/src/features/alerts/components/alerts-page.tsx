import {
  Activity03Icon,
  Add01Icon,
  Alert02Icon,
  AlertCircleIcon,
  CheckmarkCircle02Icon,
  Edit03Icon,
  Undo02Icon,
  ViewIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  keepPreviousData,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type { ColumnDef, SortingState } from '@tanstack/react-table'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/ui/data-table'
import { Spinner } from '@/components/ui/spinner'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { dashboardKeys } from '@/features/dashboard/query-keys'
import { listSites } from '@/features/sites/api'
import { siteKeys } from '@/features/sites/query-keys'
import { fromUnixSeconds } from '@/lib/dayjs'
import { translateMessageRef } from '@/lib/message-ref'
import { useAuthStore } from '@/stores/auth-store'

import { getAlertSummary, listAlertRules, listAlerts } from '../api'
import { alertRuleSortFields, alertSortFields } from '../constants'
import { alertListParams, alertRuleListParams } from '../contract'
import { alertEventTargetLabel, alertEventTargetName } from '../event-text'
import { alertKeys } from '../query-keys'
import {
  alertMessageForDisplay,
  alertRuleCategoryText,
  formatAlertCurrentValue,
  formatAlertThreshold,
} from '../rule-text'
import type {
  AlertEventItem,
  AlertRuleItem,
  AlertSearch,
  AlertSummary,
} from '../types'
import { AlertEventDetailSheet } from './alert-event-detail-sheet'
import { AlertFilters } from './alert-filters'
import { AlertRuleFormDialog, AlertRuleResetDialog } from './alert-rule-dialogs'
import { AlertRuleFilters } from './alert-rule-filters'
import {
  AlertLevelBadge,
  AlertStatusBadge,
  AlertTime,
  alertRuleDescription,
  alertRuleName,
  RuleScopeBadge,
} from './alert-ui'

type RuleDialogState = {
  action: 'edit' | 'restore'
  rule: AlertRuleItem
} | null

function SummaryStrip({ summary }: { summary?: AlertSummary }) {
  const { t } = useTranslation()
  const items = [
    {
      icon: Activity03Icon,
      iconClassName: 'bg-primary/10 text-primary',
      key: 'firing',
      label: t('alerts.summary.firing'),
      value: summary?.firing_count,
    },
    {
      icon: AlertCircleIcon,
      iconClassName: 'bg-destructive/10 text-destructive',
      key: 'critical',
      label: t('alerts.summary.critical'),
      value: summary?.critical_count,
    },
    {
      icon: Alert02Icon,
      iconClassName: 'bg-warning/10 text-warning',
      key: 'warning',
      label: t('alerts.summary.warning'),
      value: summary?.warning_count,
    },
    {
      icon: CheckmarkCircle02Icon,
      iconClassName: 'bg-success/10 text-success',
      key: 'resolved-today',
      label: t('alerts.summary.resolvedToday'),
      value: summary?.resolved_today_count,
    },
  ]
  return (
    <section aria-label={t('alerts.summary.title')}>
      <dl className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        {items.map((item) => (
          <div
            className='bg-card text-card-foreground ring-foreground/10 flex min-w-0 items-center gap-3 rounded-xl p-4 ring-1'
            key={item.key}
          >
            <span
              aria-hidden='true'
              className={`flex size-9 shrink-0 items-center justify-center rounded-lg ${item.iconClassName}`}
            >
              <HugeiconsIcon icon={item.icon} size={18} strokeWidth={2} />
            </span>
            <div className='min-w-0'>
              <dt className='text-muted-foreground truncate text-xs'>
                {item.label}
              </dt>
              <dd className='mt-0.5 text-2xl leading-none font-semibold tracking-tight tabular-nums'>
                {item.value == null ? (
                  <Spinner />
                ) : (
                  item.value.toLocaleString('zh-CN')
                )}
              </dd>
            </div>
          </div>
        ))}
      </dl>
    </section>
  )
}

function AlertEventCard({
  alert,
  onOpen,
}: {
  alert: AlertEventItem
  onOpen: (id: AlertEventItem['id']) => void
}) {
  const { t } = useTranslation()
  return (
    <article className='bg-card text-card-foreground ring-foreground/10 grid gap-4 rounded-xl p-4 ring-1'>
      <div className='flex min-w-0 items-start justify-between gap-2'>
        <div className='min-w-0'>
          <h2 className='font-semibold break-words'>
            {alertRuleName(t, alert.rule_key, alert.level)}
          </h2>
          <p className='text-muted-foreground mt-1 text-xs break-words'>
            {translateMessageRef(alertMessageForDisplay(alert.message))}
          </p>
        </div>
        <Button
          aria-label={t('alerts.detail.open')}
          onClick={() => onOpen(alert.id)}
          size='icon'
          title={t('alerts.detail.open')}
          variant='ghost'
        >
          <HugeiconsIcon icon={ViewIcon} strokeWidth={2} />
        </Button>
      </div>
      <div className='flex flex-wrap gap-2'>
        <AlertLevelBadge level={alert.level} />
        <AlertStatusBadge status={alert.status} />
      </div>
      <dl className='grid grid-cols-2 gap-3 text-sm'>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('alerts.table.site')}
          </dt>
          <dd className='break-words'>
            {alert.site_name || t('alerts.value.unavailable')}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {alertEventTargetLabel(t, alert)}
          </dt>
          <dd className='break-words'>{alertEventTargetName(t, alert)}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('alerts.table.value')}
          </dt>
          <dd>
            {t('alerts.value.currentThreshold', {
              current: alert.current_value
                ? formatAlertCurrentValue(alert.current_value)
                : t('alerts.value.unavailable'),
              threshold: alert.threshold_value
                ? formatAlertThreshold(alert.threshold_value)
                : t('alerts.value.unavailable'),
            })}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('alerts.table.lastFiredAt')}
          </dt>
          <dd>
            <AlertTime timestamp={alert.last_fired_at} />
          </dd>
        </div>
      </dl>
      <Button onClick={() => onOpen(alert.id)} variant='outline'>
        <HugeiconsIcon icon={ViewIcon} strokeWidth={2} />
        {t('alerts.detail.open')}
      </Button>
    </article>
  )
}

function RuleActions({
  onEdit,
  onRestore,
  rule,
}: {
  onEdit: (rule: AlertRuleItem) => void
  onRestore: (rule: AlertRuleItem) => void
  rule: AlertRuleItem
}) {
  const { t } = useTranslation()
  return (
    <div className='flex flex-wrap gap-2'>
      <Button onClick={() => onEdit(rule)} size='sm' variant='outline'>
        <HugeiconsIcon
          icon={rule.inherited ? Add01Icon : Edit03Icon}
          strokeWidth={2}
        />
        {rule.inherited
          ? t('alerts.rules.createOverride')
          : t('alerts.rules.edit')}
      </Button>
      {rule.override_rule_id && (
        <Button onClick={() => onRestore(rule)} size='sm' variant='ghost'>
          <HugeiconsIcon icon={Undo02Icon} strokeWidth={2} />
          {t('alerts.rules.restoreGlobal')}
        </Button>
      )}
    </div>
  )
}

function AlertRuleCard({
  isAdmin,
  onEdit,
  onRestore,
  rule,
}: {
  isAdmin: boolean
  onEdit: (rule: AlertRuleItem) => void
  onRestore: (rule: AlertRuleItem) => void
  rule: AlertRuleItem
}) {
  const { t } = useTranslation()
  return (
    <article className='bg-card text-card-foreground ring-foreground/10 grid gap-4 rounded-xl p-4 ring-1'>
      <div className='min-w-0'>
        <h2 className='font-semibold break-words'>
          {alertRuleName(t, rule.rule_key, rule.level)}
        </h2>
        <p className='text-muted-foreground mt-1 font-mono text-xs break-all'>
          {rule.rule_key}
        </p>
        <p className='text-muted-foreground mt-2 text-sm'>
          {alertRuleDescription(t, rule.rule_key)}
        </p>
      </div>
      <div className='flex flex-wrap gap-2'>
        <Badge variant='neutral'>
          {alertRuleCategoryText(t, rule.category)}
        </Badge>
        <AlertLevelBadge level={rule.level} />
        <Badge variant={rule.enabled ? 'success' : 'neutral'}>
          {rule.enabled
            ? t('alerts.rules.enabledValue')
            : t('alerts.rules.disabledValue')}
        </Badge>
        <RuleScopeBadge rule={rule} />
      </div>
      <dl className='grid grid-cols-2 gap-3 text-sm'>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('alerts.rules.metric')}
          </dt>
          <dd className='font-mono text-xs break-all'>{rule.metric}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('alerts.rules.compareOperator')}
          </dt>
          <dd className='font-mono'>{rule.compare_operator}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('alerts.rules.threshold')}
          </dt>
          <dd>
            {rule.threshold_value
              ? formatAlertThreshold(rule.threshold_value)
              : t('alerts.rules.fixedBySystem')}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('alerts.rules.forTimes')}
          </dt>
          <dd>{rule.for_times}</dd>
        </div>
      </dl>
      {isAdmin && (
        <RuleActions onEdit={onEdit} onRestore={onRestore} rule={rule} />
      )}
    </article>
  )
}

export function AlertsPage({
  onSearchChange,
  search,
}: {
  onSearchChange: (changes: Partial<AlertSearch>) => void
  search: AlertSearch
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isAdmin = useAuthStore((state) => state.user?.role === 'admin')
  const [ruleDialog, setRuleDialog] = useState<RuleDialogState>(null)
  const params = useMemo(() => alertListParams(search), [search])
  const ruleParams = useMemo(() => alertRuleListParams(search), [search])
  const sitesParams = useMemo(
    () => ({
      p: 1,
      page_size: 100,
      sort_by: 'name',
      sort_order: 'asc' as const,
    }),
    []
  )
  const sitesQuery = useQuery({
    queryFn: () => listSites(sitesParams),
    queryKey: siteKeys.list(sitesParams),
    staleTime: 5 * 60_000,
  })
  const summaryQuery = useQuery({
    enabled: search.tab === 'events',
    queryFn: getAlertSummary,
    queryKey: alertKeys.summary(),
    refetchInterval: 60_000,
    staleTime: 30_000,
  })
  const alertsQuery = useQuery({
    enabled: search.tab === 'events',
    placeholderData: keepPreviousData,
    queryFn: () => listAlerts(params),
    queryKey: alertKeys.list(params),
    refetchInterval: (query) =>
      query.state.data?.items.some((item) => item.status !== 'resolved')
        ? 60_000
        : false,
    staleTime: 30_000,
  })
  const rulesEnabled =
    search.tab === 'rules' &&
    (search.scope === 'global' || Boolean(search.ruleSiteId))
  const rulesQuery = useQuery({
    enabled: rulesEnabled,
    placeholderData: keepPreviousData,
    queryFn: () => listAlertRules(ruleParams),
    queryKey: alertKeys.rules(ruleParams),
    staleTime: 30_000,
  })
  const alerts = alertsQuery.data?.items ?? []
  const rules = rulesQuery.data?.items ?? []
  const sites = sitesQuery.data?.items ?? []
  const openAlert = useCallback(
    (alertId: AlertEventItem['id']) => onSearchChange({ alertId }),
    [onSearchChange]
  )
  const updateSorting = (
    updater: SortingState | ((old: SortingState) => SortingState)
  ) => {
    const current = search.sort
      ? [{ desc: search.order === 'desc', id: search.sort }]
      : []
    const next = typeof updater === 'function' ? updater(current) : updater
    const first = next[0]
    if (!first || !alertSortFields.includes(first.id as never)) {
      onSearchChange({ page: 1, sort: undefined })
      return
    }
    onSearchChange({
      order: first.desc ? 'desc' : 'asc',
      page: 1,
      sort: first.id as AlertSearch['sort'],
    })
  }
  const updateRuleSorting = (
    updater: SortingState | ((old: SortingState) => SortingState)
  ) => {
    const current = search.ruleSort
      ? [{ desc: search.ruleOrder === 'desc', id: search.ruleSort }]
      : []
    const next = typeof updater === 'function' ? updater(current) : updater
    const first = next[0]
    if (!first || !alertRuleSortFields.includes(first.id as never)) {
      onSearchChange({ rulePage: 1, ruleSort: undefined })
      return
    }
    onSearchChange({
      ruleOrder: first.desc ? 'desc' : 'asc',
      rulePage: 1,
      ruleSort: first.id as AlertSearch['ruleSort'],
    })
  }
  const eventColumns = useMemo<ColumnDef<AlertEventItem, unknown>[]>(
    () => [
      {
        cell: ({ row }) => (
          <button
            className='max-w-64 text-left font-medium break-words hover:underline'
            onClick={() => openAlert(row.original.id)}
            type='button'
          >
            {alertRuleName(t, row.original.rule_key, row.original.level)}
          </button>
        ),
        enableSorting: true,
        header: t('alerts.table.rule'),
        id: 'rule_key',
      },
      {
        cell: ({ row }) => <AlertLevelBadge level={row.original.level} />,
        enableSorting: true,
        header: t('alerts.table.level'),
        id: 'level',
        sortDescFirst: false,
      },
      {
        cell: ({ row }) => <AlertStatusBadge status={row.original.status} />,
        enableSorting: true,
        header: t('alerts.table.status'),
        id: 'status',
      },
      {
        accessorKey: 'site_name',
        enableSorting: true,
        header: t('alerts.table.site'),
        id: 'site_name',
        sortDescFirst: false,
      },
      {
        cell: ({ row }) => (
          <div className='min-w-32'>
            <span className='text-muted-foreground text-xs'>
              {alertEventTargetLabel(t, row.original)}
            </span>
            <p className='break-words'>
              {alertEventTargetName(t, row.original)}
            </p>
          </div>
        ),
        header: t('alerts.table.target'),
        id: 'target',
      },
      {
        cell: ({ row }) =>
          t('alerts.value.currentThreshold', {
            current: row.original.current_value
              ? formatAlertCurrentValue(row.original.current_value)
              : t('alerts.value.unavailable'),
            threshold: row.original.threshold_value
              ? formatAlertThreshold(row.original.threshold_value)
              : t('alerts.value.unavailable'),
          }),
        header: t('alerts.table.value'),
        id: 'value',
      },
      {
        cell: ({ row }) => (
          <AlertTime timestamp={row.original.first_fired_at} />
        ),
        enableSorting: true,
        header: t('alerts.table.firstFiredAt'),
        id: 'first_fired_at',
        sortDescFirst: true,
      },
      {
        cell: ({ row }) => <AlertTime timestamp={row.original.last_fired_at} />,
        enableSorting: true,
        header: t('alerts.table.lastFiredAt'),
        id: 'last_fired_at',
        sortDescFirst: true,
      },
      {
        cell: ({ row }) => <AlertTime timestamp={row.original.resolved_at} />,
        enableSorting: true,
        header: t('alerts.table.resolvedAt'),
        id: 'resolved_at',
        sortDescFirst: true,
      },
    ],
    [openAlert, t]
  )
  const ruleColumns = useMemo<ColumnDef<AlertRuleItem, unknown>[]>(
    () => [
      {
        accessorKey: 'rule_key',
        cell: ({ row }) => (
          <div className='min-w-48'>
            <p className='font-medium'>
              {alertRuleName(t, row.original.rule_key, row.original.level)}
            </p>
            <p className='text-muted-foreground font-mono text-xs break-all'>
              {row.original.rule_key}
            </p>
            <p className='text-muted-foreground mt-1 text-xs'>
              {alertRuleDescription(t, row.original.rule_key)}
            </p>
          </div>
        ),
        enableSorting: true,
        header: t('alerts.table.rule'),
      },
      {
        accessorKey: 'category',
        cell: ({ row }) => (
          <Badge variant='neutral'>
            {alertRuleCategoryText(t, row.original.category)}
          </Badge>
        ),
        enableSorting: true,
        header: t('alerts.rules.category'),
      },
      {
        accessorKey: 'level',
        cell: ({ row }) => <AlertLevelBadge level={row.original.level} />,
        enableSorting: true,
        header: t('alerts.rules.level'),
        sortDescFirst: true,
      },
      {
        accessorKey: 'metric',
        enableSorting: true,
        header: t('alerts.rules.metric'),
      },
      {
        accessorKey: 'compare_operator',
        enableSorting: false,
        header: t('alerts.rules.compareOperator'),
      },
      {
        cell: ({ row }) =>
          row.original.threshold_value
            ? formatAlertThreshold(row.original.threshold_value)
            : t('alerts.rules.fixedBySystem'),
        header: t('alerts.rules.threshold'),
        id: 'threshold',
      },
      {
        accessorKey: 'for_times',
        enableSorting: false,
        header: t('alerts.rules.forTimes'),
      },
      {
        cell: ({ row }) => <RuleScopeBadge rule={row.original} />,
        header: t('alerts.rules.scope'),
        id: 'scope',
      },
      {
        accessorKey: 'enabled',
        cell: ({ row }) => (
          <Badge variant={row.original.enabled ? 'success' : 'neutral'}>
            {row.original.enabled
              ? t('alerts.rules.enabledValue')
              : t('alerts.rules.disabledValue')}
          </Badge>
        ),
        enableSorting: true,
        header: t('alerts.rules.enabled'),
      },
      {
        accessorKey: 'updated_at',
        cell: ({ row }) => <AlertTime timestamp={row.original.updated_at} />,
        enableSorting: true,
        header: t('alerts.rules.updatedAt'),
        sortDescFirst: true,
      },
      ...(isAdmin
        ? ([
            {
              cell: ({ row }) => (
                <RuleActions
                  onEdit={(rule) => setRuleDialog({ action: 'edit', rule })}
                  onRestore={(rule) =>
                    setRuleDialog({ action: 'restore', rule })
                  }
                  rule={row.original}
                />
              ),
              header: t('common.actions'),
              id: 'actions',
            },
          ] satisfies ColumnDef<AlertRuleItem, unknown>[])
        : []),
    ],
    [isAdmin, t]
  )
  const invalidateRules = () => {
    void queryClient.invalidateQueries({ queryKey: alertKeys.all })
    void queryClient.invalidateQueries({ queryKey: dashboardKeys.health() })
  }
  return (
    <SectionPageLayout
      fixedContent
      description={t('alerts.description')}
      title={t('alerts.title')}
    >
      <div className='flex h-full min-h-0 min-w-0 flex-col gap-4'>
        <div className='flex min-w-0 items-center justify-between gap-2'>
          <Tabs
            onValueChange={(tab) =>
              onSearchChange({
                alertId: undefined,
                tab: tab as AlertSearch['tab'],
              })
            }
            value={search.tab}
          >
            <TabsList aria-label={t('alerts.tabs.label')}>
              <TabsTrigger value='events'>
                {t('alerts.tabs.events')}
              </TabsTrigger>
              <TabsTrigger value='rules'>{t('alerts.tabs.rules')}</TabsTrigger>
            </TabsList>
          </Tabs>
          {search.tab === 'events' && summaryQuery.data && (
            <p className='text-muted-foreground shrink-0 text-right text-xs tabular-nums'>
              {t('alerts.summary.updatedAt', {
                time: fromUnixSeconds(summaryQuery.data.updated_at).format(
                  'YYYY-MM-DD HH:mm:ss'
                ),
              })}
            </p>
          )}
        </div>

        {search.tab === 'events' ? (
          <div
            className='flex min-h-0 flex-1 flex-col gap-4'
            id='alerts-events-panel'
            role='tabpanel'
          >
            <SummaryStrip summary={summaryQuery.data} />
            {summaryQuery.isError && (
              <section
                className='border-warning/40 bg-warning/8 border-y p-4'
                role='alert'
              >
                <h2 className='font-medium'>{t('alerts.summary.loadError')}</h2>
                <p className='text-muted-foreground mt-1 text-sm'>
                  {t('alerts.summary.partialDescription')}
                </p>
                <Button
                  className='mt-3'
                  onClick={() => void summaryQuery.refetch()}
                  variant='outline'
                >
                  {t('common.retry')}
                </Button>
              </section>
            )}
            <AlertFilters
              onApply={(filters) => onSearchChange({ ...filters, page: 1 })}
              sites={sites}
              value={{
                end: search.end,
                level: search.level,
                siteId: search.siteId,
                start: search.start,
                status: search.status,
                targetType: search.targetType,
              }}
            />
            <DataTable
              ariaLabel={t('alerts.table.label')}
              columns={eventColumns}
              data={alerts}
              emptyDescription={t('alerts.empty.description')}
              emptyTitle={t('alerts.empty.title')}
              error={alertsQuery.isError}
              fetching={alertsQuery.isFetching}
              loading={alertsQuery.isPending}
              onPageChange={(page) => onSearchChange({ page })}
              onPageSizeChange={(pageSize) =>
                onSearchChange({ page: 1, pageSize })
              }
              onRetry={() => void alertsQuery.refetch()}
              onSortingChange={updateSorting}
              page={search.page}
              pageSize={search.pageSize}
              renderMobileCard={(alert) => (
                <AlertEventCard alert={alert} onOpen={openAlert} />
              )}
              sorting={
                search.sort
                  ? [{ desc: search.order === 'desc', id: search.sort }]
                  : []
              }
              total={alertsQuery.data?.total ?? 0}
            />
          </div>
        ) : (
          <div
            className='flex min-h-0 flex-1 flex-col gap-4'
            id='alerts-rules-panel'
            role='tabpanel'
          >
            <AlertRuleFilters
              onApply={(filters) => onSearchChange({ ...filters, rulePage: 1 })}
              sites={sites}
              value={{
                ruleCategory: search.ruleCategory,
                ruleEnabled: search.ruleEnabled,
                ruleInherited: search.ruleInherited,
                ruleLevel: search.ruleLevel,
                ruleSiteId: search.ruleSiteId,
                scope: search.scope,
              }}
            />
            {search.scope === 'site' && !search.ruleSiteId ? (
              <section className='border-border border-y py-10 text-center'>
                <h2 className='font-medium'>
                  {t('alerts.rules.siteRequired')}
                </h2>
                <p className='text-muted-foreground mt-1 text-sm'>
                  {t('alerts.rules.siteRequiredDescription')}
                </p>
              </section>
            ) : (
              <DataTable
                ariaLabel={t('alerts.rules.table')}
                columns={ruleColumns}
                data={rules}
                emptyDescription={t('alerts.rules.emptyDescription')}
                emptyTitle={t('alerts.rules.empty')}
                error={rulesQuery.isError || sitesQuery.isError}
                fetching={rulesQuery.isFetching}
                loading={rulesQuery.isPending}
                onPageChange={(rulePage) => onSearchChange({ rulePage })}
                onPageSizeChange={(rulePageSize) =>
                  onSearchChange({ rulePage: 1, rulePageSize })
                }
                onRetry={() => void rulesQuery.refetch()}
                onSortingChange={updateRuleSorting}
                page={search.rulePage}
                pageSize={search.rulePageSize}
                renderMobileCard={(rule) => (
                  <AlertRuleCard
                    isAdmin={Boolean(isAdmin)}
                    onEdit={(selected) =>
                      setRuleDialog({ action: 'edit', rule: selected })
                    }
                    onRestore={(selected) =>
                      setRuleDialog({ action: 'restore', rule: selected })
                    }
                    rule={rule}
                  />
                )}
                sorting={
                  search.ruleSort
                    ? [
                        {
                          desc: search.ruleOrder === 'desc',
                          id: search.ruleSort,
                        },
                      ]
                    : []
                }
                total={rulesQuery.data?.total ?? 0}
              />
            )}
          </div>
        )}
      </div>

      <AlertEventDetailSheet
        alertId={search.alertId}
        onClose={() => onSearchChange({ alertId: undefined })}
      />
      {ruleDialog?.action === 'edit' && (
        <AlertRuleFormDialog
          onClose={() => setRuleDialog(null)}
          onSaved={invalidateRules}
          rule={ruleDialog.rule}
          rules={rules}
          siteId={search.ruleSiteId}
        />
      )}
      {ruleDialog?.action === 'restore' && (
        <AlertRuleResetDialog
          onClose={() => setRuleDialog(null)}
          onSaved={invalidateRules}
          rule={ruleDialog.rule}
        />
      )}
    </SectionPageLayout>
  )
}
