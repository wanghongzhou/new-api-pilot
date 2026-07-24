import {
  Add01Icon,
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

import { FilterPanel } from '@/components/data/filter-panel'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/ui/data-table'
import { SelectControl as Select } from '@/components/ui/select-control'
import { Spinner } from '@/components/ui/spinner'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { dashboardKeys } from '@/features/dashboard/query-keys'
import { listSites } from '@/features/sites/api'
import { siteKeys } from '@/features/sites/query-keys'
import type { SiteListItem } from '@/features/sites/types'
import { isIdString } from '@/lib/api-types'
import { fromUnixSeconds } from '@/lib/dayjs'
import { translateMessageRef } from '@/lib/message-ref'
import { useAuthStore } from '@/stores/auth-store'

import { getAlertSummary, listAlertRules, listAlerts } from '../api'
import { alertSortFields } from '../constants'
import { alertListParams } from '../contract'
import { alertKeys } from '../query-keys'
import type {
  AlertEventItem,
  AlertRuleItem,
  AlertSearch,
  AlertSummary,
} from '../types'
import { AlertEventDetailSheet } from './alert-event-detail-sheet'
import { AlertFilters } from './alert-filters'
import { AlertRuleFormDialog, AlertRuleResetDialog } from './alert-rule-dialogs'
import {
  AlertLevelBadge,
  AlertStatusBadge,
  AlertTime,
  alertRuleDescription,
  alertRuleName,
  alertTargetTypeText,
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
      key: 'firing',
      label: t('alerts.summary.firing'),
      value: summary?.firing_count,
    },
    {
      key: 'critical',
      label: t('alerts.summary.critical'),
      value: summary?.critical_count,
    },
    {
      key: 'warning',
      label: t('alerts.summary.warning'),
      value: summary?.warning_count,
    },
    {
      key: 'resolved-today',
      label: t('alerts.summary.resolvedToday'),
      value: summary?.resolved_today_count,
    },
  ]
  return (
    <section aria-label={t('alerts.summary.title')}>
      <dl className='border-border divide-border grid divide-y border-y sm:grid-cols-2 sm:divide-x sm:divide-y-0 xl:grid-cols-4'>
        {items.map((item) => (
          <div className='min-w-0 px-4 py-3' key={item.key}>
            <dt className='text-muted-foreground text-xs'>{item.label}</dt>
            <dd className='mt-1 text-xl font-semibold tabular-nums'>
              {item.value == null ? (
                <Spinner />
              ) : (
                item.value.toLocaleString('zh-CN')
              )}
            </dd>
          </div>
        ))}
      </dl>
      {summary && (
        <p className='text-muted-foreground mt-2 text-right text-xs'>
          {t('alerts.summary.updatedAt', {
            time: fromUnixSeconds(summary.updated_at).format(
              'YYYY-MM-DD HH:mm:ss'
            ),
          })}
        </p>
      )}
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
            {alertRuleName(t, alert.rule_key)}
          </h2>
          <p className='text-muted-foreground mt-1 text-xs break-words'>
            {translateMessageRef(alert.message)}
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
            {alertTargetTypeText(t, alert.target_type)}
          </dt>
          <dd className='break-words'>{alert.target_name}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('alerts.table.value')}
          </dt>
          <dd>
            {t('alerts.value.currentThreshold', {
              current: alert.current_value ?? t('alerts.value.unavailable'),
              threshold: alert.threshold_value ?? t('alerts.value.unavailable'),
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
          {alertRuleName(t, rule.rule_key)}
        </h2>
        <p className='text-muted-foreground mt-1 font-mono text-xs break-all'>
          {rule.rule_key}
        </p>
        <p className='text-muted-foreground mt-2 text-sm'>
          {alertRuleDescription(t, rule.rule_key)}
        </p>
      </div>
      <div className='flex flex-wrap gap-2'>
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
          <dd>{rule.threshold_value ?? t('alerts.rules.fixedBySystem')}</dd>
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
    queryFn: () => listAlertRules(search.scope, search.ruleSiteId),
    queryKey: alertKeys.rules(search.scope, search.ruleSiteId),
    staleTime: 30_000,
  })
  const alerts = alertsQuery.data?.items ?? []
  const rules = rulesQuery.data ?? []
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
  const eventColumns = useMemo<ColumnDef<AlertEventItem, unknown>[]>(
    () => [
      {
        cell: ({ row }) => <AlertLevelBadge level={row.original.level} />,
        enableSorting: true,
        header: t('alerts.table.level'),
        id: 'level',
        sortDescFirst: true,
      },
      {
        cell: ({ row }) => <AlertStatusBadge status={row.original.status} />,
        enableSorting: true,
        header: t('alerts.table.status'),
        id: 'status',
        sortDescFirst: true,
      },
      {
        cell: ({ row }) => (
          <button
            className='max-w-64 text-left font-medium break-words hover:underline'
            onClick={() => openAlert(row.original.id)}
            type='button'
          >
            {alertRuleName(t, row.original.rule_key)}
          </button>
        ),
        header: t('alerts.table.rule'),
        id: 'rule',
      },
      {
        accessorKey: 'site_name',
        header: t('alerts.table.site'),
      },
      {
        cell: ({ row }) => (
          <div className='min-w-32'>
            <span className='text-muted-foreground text-xs'>
              {alertTargetTypeText(t, row.original.target_type)}
            </span>
            <p className='break-words'>{row.original.target_name}</p>
          </div>
        ),
        header: t('alerts.table.target'),
        id: 'target',
      },
      {
        cell: ({ row }) =>
          t('alerts.value.currentThreshold', {
            current:
              row.original.current_value ?? t('alerts.value.unavailable'),
            threshold:
              row.original.threshold_value ?? t('alerts.value.unavailable'),
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
        header: t('alerts.table.resolvedAt'),
        id: 'resolved_at',
      },
    ],
    [openAlert, t]
  )
  const ruleColumns = useMemo<ColumnDef<AlertRuleItem, unknown>[]>(
    () => [
      {
        cell: ({ row }) => (
          <div className='min-w-48'>
            <p className='font-medium'>
              {alertRuleName(t, row.original.rule_key)}
            </p>
            <p className='text-muted-foreground font-mono text-xs break-all'>
              {row.original.rule_key}
            </p>
            <p className='text-muted-foreground mt-1 text-xs'>
              {alertRuleDescription(t, row.original.rule_key)}
            </p>
          </div>
        ),
        header: t('alerts.table.rule'),
        id: 'rule',
      },
      {
        cell: ({ row }) => <AlertLevelBadge level={row.original.level} />,
        header: t('alerts.rules.level'),
        id: 'level',
      },
      {
        accessorKey: 'metric',
        header: t('alerts.rules.metric'),
      },
      {
        accessorKey: 'compare_operator',
        header: t('alerts.rules.compareOperator'),
      },
      {
        cell: ({ row }) =>
          row.original.threshold_value ?? t('alerts.rules.fixedBySystem'),
        header: t('alerts.rules.threshold'),
        id: 'threshold',
      },
      {
        accessorKey: 'for_times',
        header: t('alerts.rules.forTimes'),
      },
      {
        cell: ({ row }) => (
          <div className='grid gap-1'>
            <RuleScopeBadge rule={row.original} />
            <Badge variant={row.original.enabled ? 'success' : 'neutral'}>
              {row.original.enabled
                ? t('alerts.rules.enabledValue')
                : t('alerts.rules.disabledValue')}
            </Badge>
          </div>
        ),
        header: t('alerts.rules.scope'),
        id: 'scope',
      },
      {
        cell: ({ row }) => <AlertTime timestamp={row.original.updated_at} />,
        header: t('alerts.rules.updatedAt'),
        id: 'updatedAt',
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
            <TabsTrigger value='events'>{t('alerts.tabs.events')}</TabsTrigger>
            <TabsTrigger value='rules'>{t('alerts.tabs.rules')}</TabsTrigger>
          </TabsList>
        </Tabs>

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
            <FilterPanel
              description={t('alerts.rules.scopeControls')}
              hasActiveFilters={
                search.scope !== 'global' || search.ruleSiteId != null
              }
              onReset={() =>
                onSearchChange({ ruleSiteId: undefined, scope: 'global' })
              }
              title={t('alerts.rules.scopeControls')}
            >
              <div className='grid min-w-0 flex-1 gap-3 sm:grid-cols-2'>
                <label className='grid gap-1.5 text-sm'>
                  <span className='font-medium'>{t('alerts.rules.scope')}</span>
                  <Select
                    onChange={(event) =>
                      onSearchChange({
                        ruleSiteId:
                          event.target.value === 'site'
                            ? search.ruleSiteId
                            : undefined,
                        scope: event.target.value as AlertSearch['scope'],
                      })
                    }
                    value={search.scope}
                  >
                    <option value='global'>
                      {t('alerts.rules.scope.global')}
                    </option>
                    <option value='site'>{t('alerts.rules.scope.site')}</option>
                  </Select>
                </label>
                {search.scope === 'site' && (
                  <label className='grid gap-1.5 text-sm'>
                    <span className='font-medium'>
                      {t('alerts.rules.site')}
                    </span>
                    <Select
                      onChange={(event) =>
                        onSearchChange({
                          ruleSiteId: isIdString(event.target.value)
                            ? event.target.value
                            : undefined,
                        })
                      }
                      value={search.ruleSiteId ?? ''}
                    >
                      <option value=''>{t('alerts.rules.selectSite')}</option>
                      {sites.map((site: SiteListItem) => (
                        <option key={site.id} value={site.id}>
                          {site.name}
                        </option>
                      ))}
                    </Select>
                  </label>
                )}
              </div>
            </FilterPanel>
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
                onRetry={() => void rulesQuery.refetch()}
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
