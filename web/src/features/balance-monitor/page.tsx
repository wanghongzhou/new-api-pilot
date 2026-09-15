import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Main } from '@/components/layout/main'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import { CalculationDialog } from './calculation-dialog'
import {
  accounts,
  amount,
  balanceSearch,
  changeBalanceView,
  normalizeBalanceSearch,
  records,
  sampleTime,
  summarize,
  validBalanceDates,
  type BalanceSearch,
  type RecordItem,
} from './data'
import { MonthlySnapshots } from './monthly-snapshots'
import { BalanceRecordTable } from './record-table'

export function BalanceMonitorPage({
  search,
  onChange,
}: {
  search: BalanceSearch
  onChange: (value: BalanceSearch) => void
}) {
  const { t } = useTranslation()
  const [detail, setDetail] = useState<RecordItem | null>(null)
  const [dateError, setDateError] = useState(false)
  const normalized = normalizeBalanceSearch(search)
  const current = search.kind === 'current'
  const monthly = search.kind === 'monthly'
  const detailKind = search.kind === 'monthly' ? null : search.kind
  const interval = search.kind === 'interval'
  const validDates = validBalanceDates(normalized)
  const form = useForm<BalanceSearch>({
    resolver: zodResolver(balanceSearch),
    values: normalized,
  })
  const directory = useQuery({
    queryKey: ['balance-monitor', 'accounts'],
    queryFn: accounts,
    enabled: !monthly,
    refetchInterval: 60000,
  })
  const query = useQuery({
    queryKey: ['balance-monitor', normalized],
    queryFn: () => records({ ...normalized, page_size: 50 }),
    refetchInterval: 60000,
    enabled: validDates && !monthly,
  })
  const inventory = useQuery({
    queryKey: ['balance-monitor', 'inventory'],
    queryFn: () => records({ kind: 'inventory', page_size: 200 }),
    enabled: current,
    refetchInterval: current ? 60000 : false,
  })
  const rows = query.data?.items ?? []
  const summary = summarize(rows, current ? 'balance_yuan' : 'consumption_yuan')
  const sites = [
    ...new Map(
      (directory.data?.items ?? []).map((row) => [row.site_id, row.site_name])
    ).entries(),
  ]
  const chosenSite = form.watch('site_id')
  const visibleAccounts = (directory.data?.items ?? []).filter(
    (row) => !chosenSite || row.site_id === chosenSite
  )
  const title = {
    current: t('Latest account balances'),
    interval: t('Consumption intervals table'),
    daily: t('Daily consumption reports'),
    monthly: t('Monthly balance snapshots'),
  }[search.kind]
  const description = {
    current: t('Current balance view explanation'),
    interval: t('Consumption interval view explanation'),
    daily: t('Daily report view explanation'),
    monthly: t('Monthly snapshot explanation'),
  }[search.kind]
  const emptyMessage = {
    current: t('No current balances yet'),
    interval: t('No intervals in selected dates'),
    daily: t('No daily reports yet'),
    monthly: t('No monthly snapshots'),
  }[search.kind]
  const update = (value: BalanceSearch) => {
    setDetail(null)
    setDateError(false)
    onChange(value)
  }
  const refresh = () => {
    if (validDates) void query.refetch()
    void directory.refetch()
    if (current) void inventory.refetch()
  }
  return (
    <Main className='overflow-y-auto p-4 md:p-6'>
      <div className='space-y-5'>
        <header>
          <h1 className='text-2xl font-semibold'>
            {t('Balance and consumption')}
          </h1>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('Balance monitoring navigation hint')}
          </p>
        </header>
        <div
          className='flex flex-wrap gap-2 border-b pb-4'
          aria-label={t('Balance views')}
        >
          <Button
            className='min-h-10'
            aria-pressed={current}
            variant={current ? 'default' : 'outline'}
            onClick={() => update(changeBalanceView(normalized, 'current'))}
          >
            {t('Current balances')}
          </Button>
          <Button
            className='min-h-10'
            aria-pressed={interval}
            variant={interval ? 'default' : 'outline'}
            onClick={() => update(changeBalanceView(normalized, 'interval'))}
          >
            {t('Consumption details')}
          </Button>
          <Button
            className='min-h-10'
            aria-pressed={search.kind === 'daily'}
            variant={search.kind === 'daily' ? 'default' : 'outline'}
            onClick={() => update(changeBalanceView(normalized, 'daily'))}
          >
            {t('Daily consumption reports')}
          </Button>
          <Button
            className='min-h-10'
            aria-pressed={monthly}
            variant={monthly ? 'default' : 'outline'}
            onClick={() => update(changeBalanceView(normalized, 'monthly'))}
          >
            {t('Monthly balance snapshots')}
          </Button>
        </div>
        {monthly && <MonthlySnapshots search={normalized} onChange={update} />}
        {detailKind && (
          <section aria-label={title} className='space-y-4'>
            <div>
              <h2 className='text-lg font-semibold'>{title}</h2>
              <p className='text-muted-foreground mt-1 max-w-4xl text-sm leading-relaxed'>
                {description}
              </p>
            </div>
            {current && (
              <aside
                aria-label={t('Current redeem stock')}
                className='bg-muted/50 flex flex-wrap items-center gap-x-6 gap-y-2 rounded-lg px-4 py-3 text-sm'
              >
                <span className='font-medium'>{t('Current redeem stock')}</span>
                {inventory.isPending && (
                  <span role='status'>{t('Loading')}</span>
                )}
                {!inventory.isPending && inventory.isError && (
                  <span role='alert'>{t('Stock unavailable')}</span>
                )}
                {!inventory.isPending &&
                  !inventory.isError &&
                  !inventory.data?.items.length && (
                    <span className='text-muted-foreground'>
                      {t('No stock snapshot')}
                    </span>
                  )}
                {!inventory.isPending &&
                  !inventory.isError &&
                  inventory.data?.items.map((row) => (
                    <div
                      key={row.source_id}
                      className='flex flex-wrap items-center gap-x-4 gap-y-1'
                    >
                      <span className='tabular-nums'>
                        {t('Redeem stock count', {
                          count: row.balance?.split('.')[0] ?? '—',
                        })}
                      </span>
                      <span className='tabular-nums'>
                        {t('Stock value CNY', {
                          value: amount(row.balance_yuan),
                        })}
                      </span>
                      <span className='text-muted-foreground text-xs'>
                        {sampleTime(row.sampled_at)}
                      </span>
                    </div>
                  ))}
              </aside>
            )}
            <form
              className='flex flex-wrap items-end gap-3'
              onSubmit={form.handleSubmit((value) => {
                if (!validBalanceDates(value)) {
                  setDateError(true)
                  return
                }
                update({ ...value, p: 1 })
              })}
            >
              <label className='grid min-w-0 gap-1 text-sm'>
                {t('Site')}
                <select
                  className='bg-background h-10 max-w-64 rounded-md border px-3'
                  {...form.register('site_id', {
                    onChange: () => form.setValue('account_id', ''),
                  })}
                >
                  <option value=''>{t('All sites')}</option>
                  {sites.map(([id, name]) => (
                    <option key={id} value={id}>
                      {name}
                    </option>
                  ))}
                </select>
              </label>
              <label className='grid min-w-0 gap-1 text-sm'>
                {t('Account')}
                <select
                  className='bg-background h-10 max-w-64 rounded-md border px-3'
                  {...form.register('account_id')}
                >
                  <option value=''>{t('All accounts')}</option>
                  {visibleAccounts.map((row) => (
                    <option key={row.account_id} value={row.account_id}>
                      {row.name}
                    </option>
                  ))}
                </select>
              </label>
              {!current && (
                <>
                  <label className='grid gap-1 text-sm'>
                    {t('Start date')}
                    <Input
                      className='h-10'
                      type='date'
                      required
                      {...form.register('start')}
                    />
                  </label>
                  <label className='grid gap-1 text-sm'>
                    {t('End date')}
                    <Input
                      className='h-10'
                      type='date'
                      required
                      {...form.register('end')}
                    />
                  </label>
                </>
              )}
              <Button className='min-h-10' type='submit'>
                {t('Query')}
              </Button>
              <Button
                className='min-h-10'
                variant='outline'
                type='button'
                onClick={refresh}
              >
                {t('Refresh')}
              </Button>
            </form>
            {(dateError || !validDates) && (
              <p role='alert' className='text-destructive text-sm'>
                {t('Invalid balance date range')}
              </p>
            )}
            {directory.isError && (
              <p role='alert' className='text-muted-foreground text-sm'>
                {t('Account filters unavailable')}
              </p>
            )}
            {validDates && (
              <>
                {query.isPending && (
                  <p
                    className='text-muted-foreground py-8 text-center'
                    role='status'
                  >
                    {t('Loading')}
                  </p>
                )}
                {query.isError && (
                  <p className='rounded-lg border p-4 text-sm' role='alert'>
                    {t('Balance loading failed')}
                  </p>
                )}
                {!query.isPending && !query.isError && !rows.length && (
                  <div className='text-muted-foreground rounded-xl border border-dashed p-8 text-center text-sm'>
                    {emptyMessage}
                  </div>
                )}
                {!!rows.length && (
                  <>
                    <BalanceRecordTable
                      kind={detailKind}
                      rows={rows}
                      onDetail={setDetail}
                      onHistory={(row) =>
                        update({
                          ...changeBalanceView(normalized, 'interval'),
                          site_id: row.site_id,
                          account_id: row.account_id,
                        })
                      }
                    />
                    {!query.isError && (
                      <footer className='bg-muted/40 flex flex-wrap gap-x-6 gap-y-2 rounded-lg px-4 py-3 text-sm'>
                        <span className='font-medium'>
                          {current
                            ? t('Visible balances subtotal')
                            : t('Visible consumption subtotal')}
                        </span>
                        <span className='tabular-nums'>
                          {t('Noncorporate subtotal CNY', {
                            value: summary.total ?? '—',
                          })}
                        </span>
                        <span className='tabular-nums'>
                          {t('Corporate subtotal CNY', {
                            value: summary.standalone ?? '—',
                          })}
                        </span>
                        <span className='text-muted-foreground'>
                          {t('Unknown excluded count', {
                            count: summary.unknown,
                          })}
                        </span>
                      </footer>
                    )}
                  </>
                )}
                {query.data && (
                  <nav
                    aria-label={t('Balance pagination')}
                    className='flex flex-wrap items-center justify-between gap-3'
                  >
                    <p className='text-muted-foreground text-sm'>
                      {t('Balance record count', {
                        count: query.data.total,
                        page: normalized.p,
                      })}
                    </p>
                    <div className='flex gap-2'>
                      <Button
                        className='min-h-10'
                        variant='outline'
                        disabled={normalized.p <= 1}
                        onClick={() =>
                          update({ ...normalized, p: normalized.p - 1 })
                        }
                      >
                        {t('Previous page')}
                      </Button>
                      <Button
                        className='min-h-10'
                        variant='outline'
                        disabled={
                          BigInt(normalized.p * 50) >= BigInt(query.data.total)
                        }
                        onClick={() =>
                          update({ ...normalized, p: normalized.p + 1 })
                        }
                      >
                        {t('Next page')}
                      </Button>
                    </div>
                  </nav>
                )}
              </>
            )}
          </section>
        )}
        <CalculationDialog row={detail} onClose={() => setDetail(null)} />
      </div>
    </Main>
  )
}
