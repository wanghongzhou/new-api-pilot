import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import {
  amount,
  balanceDeltaYuan,
  coveragePercent,
  intervalMinutes,
  sampleTime,
  type BalanceSearch,
  type RecordItem,
} from './data'

export function OriginalUnit({ field }: { field?: string }) {
  const { t } = useTranslation()
  if (field === 'quota') return <>{t('Quota unit')}</>
  if (field === 'quota+credit_limit') return <>{t('Quota with credit unit')}</>
  return <>{t('Balance unit')}</>
}

export function RecordStatus({ row }: { row: RecordItem }) {
  const { t } = useTranslation()
  let flags = row.flags
  if (row.kind === 'current') {
    flags = row.balance == null ? ['unavailable'] : []
  }
  const labels = flags.map((flag) => {
    switch (flag) {
      case 'baseline':
        return t('First sample')
      case 'unavailable':
        return t('Collection unavailable')
      case 'gap':
        return t('Long sampling interval')
      case 'recharge_unknown':
        return t('Recharge unverified')
      case 'balance_increase':
        return t('Unexplained balance increase')
      case 'allocated':
        return t('Allocated across midnight')
      case 'partial':
        return t('Incomplete coverage')
      case 'anomaly':
        return t('Contains anomalies')
      default:
        return flag
    }
  })
  if (
    row.kind === 'current' &&
    Date.now() / 1000 - Number(row.sampled_at) > 1800
  ) {
    labels.push(t('Stale balance'))
  }
  return (
    <span className={labels.length ? 'text-muted-foreground' : ''}>
      {labels.length ? labels.join(' · ') : t('Normal')}
    </span>
  )
}

function AccountCell({ row }: { row: RecordItem }) {
  const { t } = useTranslation()
  return (
    <td className='p-3'>
      <span className='font-medium'>{row.name}</span>
      {row.standalone && (
        <span className='ml-2 rounded border px-1.5 text-xs'>
          {t('Corporate account')}
        </span>
      )}
      <span className='text-muted-foreground block text-xs'>
        {row.site_name}
      </span>
    </td>
  )
}

export function BalanceRecordTable({
  kind,
  rows,
  onDetail,
  onHistory,
}: {
  kind: Exclude<BalanceSearch['kind'], 'monthly'>
  rows: RecordItem[]
  onDetail: (row: RecordItem) => void
  onHistory: (row: RecordItem) => void
}) {
  const { t } = useTranslation()
  const isCurrent = kind === 'current'
  const isInterval = kind === 'interval'
  const caption = {
    current: t('Latest account balances'),
    interval: t('Consumption intervals table'),
    daily: t('Daily consumption reports'),
  }[kind]
  const action = {
    current: t('View account consumption'),
    interval: t('View calculation'),
    daily: t('Details'),
  }[kind]
  return (
    <div className='overflow-x-auto rounded-xl border'>
      <table className='w-full text-sm'>
        <caption className='sr-only'>{caption}</caption>
        <thead className='bg-muted/60 text-left'>
          <tr>
            <th className='p-3'>{t('Account')}</th>
            {isCurrent && (
              <>
                <th className='p-3 text-right'>{t('Current balance CNY')}</th>
                <th className='p-3'>{t('Raw account balance')}</th>
                <th className='p-3'>{t('Balance updated at')}</th>
              </>
            )}
            {isInterval && (
              <>
                <th className='p-3'>{t('Compared sampling times')}</th>
                <th className='p-3'>{t('Actual interval')}</th>
                <th className='p-3 text-right'>
                  {t('Balance difference CNY')}
                </th>
                <th className='p-3 text-right'>
                  {t('Adjusted consumption CNY')}
                </th>
              </>
            )}
            {kind === 'daily' && (
              <>
                <th className='p-3'>{t('Report day')}</th>
                <th className='p-3 text-right'>{t('Daily consumption CNY')}</th>
                <th className='p-3'>{t('Effective day coverage')}</th>
              </>
            )}
            <th className='p-3'>
              {isCurrent ? t('Balance availability') : t('Data quality')}
            </th>
            <th className='p-3'>
              <span className='sr-only'>{t('Details')}</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr
              key={`${row.source_id}-${row.record_id}`}
              className='hover:bg-muted/20 border-t'
            >
              <AccountCell row={row} />
              {isCurrent && (
                <>
                  <td className='p-3 text-right font-medium tabular-nums'>
                    {amount(row.balance_yuan)}
                  </td>
                  <td className='p-3 tabular-nums'>
                    {amount(row.balance)}
                    <span className='text-muted-foreground block text-xs'>
                      <OriginalUnit field={row.field} />
                    </span>
                  </td>
                  <td className='p-3 whitespace-nowrap tabular-nums'>
                    {sampleTime(row.sampled_at)}
                  </td>
                </>
              )}
              {isInterval && (
                <>
                  <td className='p-3 text-xs whitespace-nowrap tabular-nums'>
                    {row.previous_balance == null ? (
                      <span>{t('No previous sample')}</span>
                    ) : (
                      <span className='text-muted-foreground block'>
                        {sampleTime(row.started_at)}
                      </span>
                    )}
                    <span className='block'>
                      {t('Interval ends at', {
                        time: sampleTime(row.sampled_at),
                      })}
                    </span>
                  </td>
                  <td className='p-3 whitespace-nowrap tabular-nums'>
                    {intervalMinutes(row) == null
                      ? '—'
                      : t('Interval minutes', {
                          minutes: intervalMinutes(row),
                        })}
                  </td>
                  <td className='p-3 text-right tabular-nums'>
                    {amount(balanceDeltaYuan(row))}
                  </td>
                  <td className='p-3 text-right font-medium tabular-nums'>
                    {amount(row.consumption_yuan)}
                  </td>
                </>
              )}
              {kind === 'daily' && (
                <>
                  <td className='p-3 whitespace-nowrap'>{row.day}</td>
                  <td className='p-3 text-right font-medium tabular-nums'>
                    {amount(row.consumption_yuan)}
                  </td>
                  <td className='p-3 tabular-nums'>{coveragePercent(row)}</td>
                </>
              )}
              <td className='max-w-52 p-3'>
                <RecordStatus row={row} />
              </td>
              <td className='p-3'>
                <Button
                  className='min-h-10 whitespace-nowrap'
                  variant='ghost'
                  onClick={() => (isCurrent ? onHistory(row) : onDetail(row))}
                >
                  {action}
                </Button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
