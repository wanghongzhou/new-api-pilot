import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import {
  amount,
  balanceSearch,
  records,
  sampleTime,
  validBalanceDates,
  type BalanceSearch,
  type RecordItem,
} from './data'

export function MonthlyReportView({ row }: { row: RecordItem }) {
  const { t } = useTranslation()
  const report = row.monthly
  if (!report) return null
  const mode = {
    scheduled: t('Scheduled monthly snapshot'),
    manual: t('Manual monthly snapshot'),
    legacy: t('Legacy monthly snapshot'),
  }[report.mode]
  return (
    <article className='space-y-4' aria-label={t('Monthly snapshot result')}>
      <header className='flex flex-wrap items-center justify-between gap-3'>
        <div>
          <h3 className='text-lg font-semibold'>
            {t('Snapshot month title', { month: report.period })}
          </h3>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t('Snapshot actual time', { time: sampleTime(row.sampled_at) })} ·{' '}
            {mode}
          </p>
        </div>
        <span className='rounded-md border px-3 py-1 text-sm'>
          {report.failed.length
            ? t('Snapshot partial')
            : t('Snapshot collected')}
        </span>
      </header>
      <div className='grid gap-3 sm:grid-cols-3'>
        <section className='rounded-xl border p-4'>
          <p className='text-muted-foreground text-sm'>
            {t('Monthly total residual')}
          </p>
          <p className='mt-2 text-xl font-semibold tabular-nums'>
            {amount(report.grand_total_residual_yuan)}
          </p>
        </section>
        <section className='rounded-xl border p-4'>
          <p className='text-muted-foreground text-sm'>
            {t('Monthly excluding corporate')}
          </p>
          <p className='mt-2 text-xl font-semibold tabular-nums'>
            {amount(report.total_excluding_nowcoding_residual_yuan)}
          </p>
        </section>
        <section className='rounded-xl border p-4'>
          <p className='text-muted-foreground text-sm'>
            {t('Monthly stock residual')}
          </p>
          <p className='mt-2 text-xl font-semibold tabular-nums'>
            {amount(report.redeem_code_residual_yuan)}
          </p>
          <p className='text-muted-foreground text-sm'>
            {t('Redeem stock count', { count: report.redeem_code_count })}
          </p>
        </section>
      </div>
      <dl className='bg-muted/40 flex flex-wrap gap-x-8 gap-y-2 rounded-lg px-4 py-3 text-sm'>
        <div>
          <dt className='text-muted-foreground inline'>
            {t('Monthly foxcode residual')}
          </dt>
          <dd className='ml-2 inline tabular-nums'>
            {amount(report.foxcode_residual_yuan)}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground inline'>
            {t('Monthly other residual')}
          </dt>
          <dd className='ml-2 inline tabular-nums'>
            {amount(report.other_residual_yuan)}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground inline'>
            {t('Monthly corporate residual')}
          </dt>
          <dd className='ml-2 inline tabular-nums'>
            {amount(report.nowcoding_residual_yuan)}
          </dd>
        </div>
      </dl>
      <p className='text-muted-foreground text-sm'>
        {t('Frozen monthly amounts notice')}
      </p>
      {!!report.failed.length && (
        <div role='status' className='rounded-lg border p-3 text-sm'>
          <p className='font-medium'>
            {t('Monthly failed accounts', { count: report.failed.length })}
          </p>
          <p className='text-muted-foreground mt-1 break-words'>
            {report.failed.join('、')}
          </p>
        </div>
      )}
      <div className='overflow-x-auto rounded-xl border'>
        <table className='w-full text-sm'>
          <caption className='sr-only'>
            {t('Monthly snapshot account balances')}
          </caption>
          <thead className='bg-muted/60 text-left'>
            <tr>
              <th className='p-3'>{t('Account')}</th>
              <th className='p-3 text-right'>{t('Raw account balance')}</th>
              <th className='p-3 text-right'>{t('Snapshot residual CNY')}</th>
            </tr>
          </thead>
          <tbody>
            {report.accounts.map((account) => (
              <tr key={account.name} className='border-t'>
                <td className='p-3'>{account.name}</td>
                <td className='p-3 text-right tabular-nums'>
                  {amount(account.raw)}
                </td>
                <td className='p-3 text-right tabular-nums'>
                  {amount(account.residual_yuan)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </article>
  )
}

export function MonthlySnapshots({
  search,
  onChange,
}: {
  search: BalanceSearch
  onChange: (value: BalanceSearch) => void
}) {
  const { t } = useTranslation()
  const [selected, setSelected] = useState('')
  const [invalid, setInvalid] = useState(false)
  const valid = validBalanceDates(search)
  const form = useForm<BalanceSearch>({
    resolver: zodResolver(balanceSearch),
    values: search,
  })
  const query = useQuery({
    queryKey: [
      'balance-monitor',
      'monthly',
      search.start,
      search.end,
      search.p,
    ],
    queryFn: () =>
      records({
        kind: 'monthly',
        start: `${search.start}-01`,
        end: `${search.end}-01`,
        p: search.p,
        page_size: 12,
      }),
    enabled: valid,
    refetchInterval: 60000,
  })
  const items = query.data?.items ?? []
  const active =
    items.find((row) => `${row.source_id}/${row.record_id}` === selected) ??
    items[0]
  const change = (value: BalanceSearch) => {
    setSelected('')
    setInvalid(false)
    onChange(value)
  }
  return (
    <section className='space-y-4' aria-label={t('Monthly balance snapshots')}>
      <div>
        <h2 className='text-lg font-semibold'>
          {t('Monthly balance snapshots')}
        </h2>
        <p className='text-muted-foreground mt-1 max-w-4xl text-sm'>
          {t('Monthly snapshot explanation')}
        </p>
      </div>
      <form
        className='flex flex-wrap items-end gap-3'
        onSubmit={form.handleSubmit((value) => {
          if (!validBalanceDates(value)) {
            setInvalid(true)
            return
          }
          change({ ...value, p: 1 })
        })}
      >
        <label className='grid gap-1 text-sm'>
          {t('Start month')}
          <Input type='month' required {...form.register('start')} />
        </label>
        <label className='grid gap-1 text-sm'>
          {t('End month')}
          <Input type='month' required {...form.register('end')} />
        </label>
        <Button type='submit'>{t('Query')}</Button>
        <Button
          type='button'
          variant='outline'
          disabled={!valid}
          onClick={() => void query.refetch()}
        >
          {t('Refresh')}
        </Button>
      </form>
      {(invalid || !valid) && (
        <p role='alert' className='text-destructive text-sm'>
          {t('Invalid monthly range')}
        </p>
      )}
      {valid && query.isPending && <p role='status'>{t('Loading')}</p>}
      {query.isError && <p role='alert'>{t('Balance loading failed')}</p>}
      {valid && !query.isPending && !query.isError && !items.length && (
        <p className='text-muted-foreground rounded-xl border border-dashed p-8 text-center text-sm'>
          {t('No monthly snapshots')}
        </p>
      )}
      {!!items.length && (
        <label className='flex flex-wrap items-center gap-3 text-sm'>
          {t('Select monthly snapshot')}
          <select
            className='bg-background h-10 rounded-md border px-3'
            value={active ? `${active.source_id}/${active.record_id}` : ''}
            onChange={(event) => setSelected(event.target.value)}
          >
            {items.map((row) => (
              <option
                key={`${row.source_id}-${row.record_id}`}
                value={`${row.source_id}/${row.record_id}`}
              >
                {row.monthly?.period} · {sampleTime(row.sampled_at)}
              </option>
            ))}
          </select>
        </label>
      )}
      {active && <MonthlyReportView row={active} />}
      {query.data && (
        <nav
          className='flex flex-wrap items-center justify-between gap-3'
          aria-label={t('Balance pagination')}
        >
          <p className='text-muted-foreground text-sm'>
            {t('Monthly report count', {
              count: query.data.total,
              page: search.p,
            })}
          </p>
          <div className='flex gap-2'>
            <Button
              variant='outline'
              disabled={search.p <= 1}
              onClick={() => change({ ...search, p: search.p - 1 })}
            >
              {t('Previous page')}
            </Button>
            <Button
              variant='outline'
              disabled={BigInt(search.p * 12) >= BigInt(query.data.total)}
              onClick={() => change({ ...search, p: search.p + 1 })}
            >
              {t('Next page')}
            </Button>
          </div>
        </nav>
      )}
    </section>
  )
}
