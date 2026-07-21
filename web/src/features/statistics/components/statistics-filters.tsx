import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { FilterPanel } from '@/components/data/filter-panel'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { listAccounts } from '@/features/accounts/api'
import { accountKeys } from '@/features/accounts/query-keys'
import { listCustomers } from '@/features/customers/api'
import { customerKeys } from '@/features/customers/query-keys'
import { listSites } from '@/features/sites/api'
import { siteKeys } from '@/features/sites/query-keys'
import { parseIdString } from '@/lib/api-types'

import {
  listChannelOptions,
  listGroupOptions,
  listModelOptions,
  listNodeOptions,
  listTokenOptions,
} from '../api'
import { statisticsKeys } from '../query-keys'
import type { StatisticsScope, StatisticsSearch } from '../types'

const filterSchema = z.object({
  accountIds: z.array(z.string().regex(/^[1-9]\d*$/)).max(100),
  channelKeys: z.array(z.string().regex(/^[1-9]\d*:(?:0|[1-9]\d*)$/)).max(100),
  customerIds: z.array(z.string().regex(/^[1-9]\d*$/)).max(100),
  models: z.array(z.string().min(1).max(255)).max(100),
  nodeNames: z.array(z.string().max(128)).max(100),
  siteIds: z.array(z.string().regex(/^[1-9]\d*$/)).max(100),
  tokenKeys: z.array(z.string().regex(/^[1-9]\d*:(?:0|[1-9]\d*)$/)).max(100),
  useGroups: z.array(z.string().max(128)).max(100),
})

type FilterValues = z.infer<typeof filterSchema>

interface FilterOption {
  label: string
  value: string
}

function useDebouncedValue(value: string, delay: number) {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delay)
    return () => window.clearTimeout(timer)
  }, [delay, value])
  return debounced
}

function uniqueOptions(
  options: readonly FilterOption[],
  selected: readonly string[]
) {
  const result = new Map(options.map((option) => [option.value, option]))
  for (const value of selected) {
    if (!result.has(value)) result.set(value, { label: value, value })
  }
  return [...result.values()]
}

function FilterGroup({
  emptyLabel,
  label,
  onChange,
  options,
  values,
}: {
  emptyLabel: string
  label: string
  onChange: (values: string[]) => void
  options: FilterOption[]
  values: string[]
}) {
  return (
    <fieldset className='grid min-w-0 gap-2'>
      <legend className='text-sm font-medium'>{label}</legend>
      <div className='border-border max-h-52 overflow-y-auto rounded-md border p-1'>
        {options.length === 0 ? (
          <p className='text-muted-foreground px-2 py-4 text-sm'>
            {emptyLabel}
          </p>
        ) : (
          options.map((option) => (
            <label
              className='hover:bg-muted flex min-h-10 min-w-0 items-start gap-2 rounded-md px-2 py-2 text-sm'
              key={option.value}
            >
              <Checkbox
                checked={values.includes(option.value)}
                className='mt-0.5'
                onCheckedChange={() =>
                  onChange(
                    values.includes(option.value)
                      ? values.filter((value) => value !== option.value)
                      : [...values, option.value]
                  )
                }
              />
              <span className='min-w-0 break-words'>{option.label}</span>
            </label>
          ))
        )}
      </div>
    </fieldset>
  )
}

function supports(scope: StatisticsScope, filter: keyof FilterValues) {
  if (filter === 'siteIds') return true
  if (filter === 'customerIds') {
    return scope === 'customer' || scope === 'account'
  }
  if (filter === 'accountIds') return scope === 'account'
  if (filter === 'models') return scope === 'model'
  if (filter === 'channelKeys') return scope === 'channel'
  if (filter === 'useGroups') return scope === 'group'
  if (filter === 'tokenKeys') return scope === 'token'
  return scope === 'node'
}

export function StatisticsFilters({
  onApply,
  scope,
  search,
}: {
  onApply: (changes: Partial<StatisticsSearch>) => void
  scope: StatisticsScope
  search: StatisticsSearch
}) {
  const { t } = useTranslation()
  const [keyword, setKeyword] = useState('')
  const debouncedKeyword = useDebouncedValue(keyword.trim(), 500)
  const form = useForm<FilterValues>({
    defaultValues: {
      accountIds: search.accountIds,
      channelKeys: search.channelKeys,
      customerIds: search.customerIds,
      models: search.models,
      nodeNames: search.nodeNames,
      siteIds: search.siteIds,
      tokenKeys: search.tokenKeys,
      useGroups: search.useGroups,
    },
    resolver: zodResolver(filterSchema),
  })
  useEffect(() => {
    form.reset({
      accountIds: search.accountIds,
      channelKeys: search.channelKeys,
      customerIds: search.customerIds,
      models: search.models,
      nodeNames: search.nodeNames,
      siteIds: search.siteIds,
      tokenKeys: search.tokenKeys,
      useGroups: search.useGroups,
    })
  }, [form, search])

  const siteParams = useMemo(() => ({ p: 1, page_size: 100 }), [])
  const customerParams = useMemo(() => ({ p: 1, page_size: 100 }), [])
  const accountParams = useMemo(() => ({ p: 1, page_size: 100 }), [])
  const siteQuery = useQuery({
    enabled: true,
    queryFn: () => listSites(siteParams),
    queryKey: siteKeys.list(siteParams),
    staleTime: 5 * 60_000,
  })
  const customerQuery = useQuery({
    enabled: supports(scope, 'customerIds'),
    queryFn: () => listCustomers(customerParams),
    queryKey: customerKeys.list(customerParams),
    staleTime: 5 * 60_000,
  })
  const accountQuery = useQuery({
    enabled: supports(scope, 'accountIds'),
    queryFn: () => listAccounts(accountParams),
    queryKey: accountKeys.list(accountParams),
    staleTime: 5 * 60_000,
  })
  const siteIds = form.watch('siteIds')
  const optionParams = useMemo(
    () => ({
      keyword: debouncedKeyword || undefined,
      p: 1,
      page_size: 50,
      site_ids: siteIds.map(parseIdString),
    }),
    [debouncedKeyword, siteIds]
  )
  const modelQuery = useQuery({
    enabled: scope === 'model',
    queryFn: () => listModelOptions(optionParams),
    queryKey: statisticsKeys.options('models', optionParams),
    staleTime: 5 * 60_000,
  })
  const channelQuery = useQuery({
    enabled: scope === 'channel',
    queryFn: () => listChannelOptions(optionParams),
    queryKey: statisticsKeys.options('channels', optionParams),
    staleTime: 5 * 60_000,
  })
  const groupQuery = useQuery({
    enabled: scope === 'group',
    queryFn: () => listGroupOptions(optionParams),
    queryKey: statisticsKeys.options('groups', optionParams),
    staleTime: 5 * 60_000,
  })
  const tokenQuery = useQuery({
    enabled: scope === 'token',
    queryFn: () => listTokenOptions(optionParams),
    queryKey: statisticsKeys.options('tokens', optionParams),
    staleTime: 5 * 60_000,
  })
  const nodeQuery = useQuery({
    enabled: scope === 'node',
    queryFn: () => listNodeOptions(optionParams),
    queryKey: statisticsKeys.options('nodes', optionParams),
    staleTime: 5 * 60_000,
  })

  const customerIds = form.watch('customerIds')
  const accountIds = form.watch('accountIds')
  const models = form.watch('models')
  const channelKeys = form.watch('channelKeys')
  const useGroups = form.watch('useGroups')
  const tokenKeys = form.watch('tokenKeys')
  const nodeNames = form.watch('nodeNames')
  const count =
    siteIds.length +
    customerIds.length +
    accountIds.length +
    models.length +
    channelKeys.length +
    useGroups.length +
    tokenKeys.length +
    nodeNames.length
  const setValues = (key: keyof FilterValues, values: string[]) =>
    form.setValue(key, values, { shouldDirty: true, shouldValidate: true })

  const siteOptions = uniqueOptions(
    (siteQuery.data?.items ?? []).map((site) => ({
      label: t('statistics.filter.namedId', { id: site.id, name: site.name }),
      value: site.id,
    })),
    siteIds
  )
  const customerOptions = uniqueOptions(
    (customerQuery.data?.items ?? []).map((customer) => ({
      label: t('statistics.filter.namedId', {
        id: customer.id,
        name: customer.name,
      }),
      value: customer.id,
    })),
    customerIds
  )
  const accountOptions = uniqueOptions(
    (accountQuery.data?.items ?? []).map((account) => ({
      label: t('statistics.filter.namedId', {
        id: account.id,
        name: account.username,
      }),
      value: account.id,
    })),
    accountIds
  )
  const modelOptions = uniqueOptions(
    (modelQuery.data?.items ?? []).map((model) => ({
      label: t('statistics.filter.modelOption', {
        model: model.model_name,
        site: model.site_name,
      }),
      value: model.model_name,
    })),
    models
  )
  const channelOptions = uniqueOptions(
    (channelQuery.data?.items ?? []).map((channel) => ({
      label: t('statistics.filter.channelOption', {
        channel: channel.name,
        id: channel.remote_channel_id,
        site: channel.site_name,
      }),
      value: channel.key,
    })),
    channelKeys
  )
  const groupOptions = uniqueOptions(
    (groupQuery.data?.items ?? []).map((group) => ({
      label: t('statistics.filter.groupOption', {
        group: group.use_group || t('statistics.group.unknown'),
        site: group.site_name,
      }),
      value: group.use_group,
    })),
    useGroups
  )
  const tokenOptions = uniqueOptions(
    (tokenQuery.data?.items ?? []).map((token) => ({
      label: t('statistics.filter.tokenOption', {
        id: token.token_id,
        name:
          token.token_name ||
          (token.token_id === '0'
            ? t('statistics.token.unknownDeleted')
            : t('statistics.token.unnamed')),
        site: token.site_name,
      }),
      value: token.key,
    })),
    tokenKeys
  )
  const nodeOptions = uniqueOptions(
    (nodeQuery.data?.items ?? []).map((node) => ({
      label: t('statistics.filter.nodeOption', {
        node: node.node_name || t('statistics.node.unknown'),
        site: node.site_name,
      }),
      value: node.node_name,
    })),
    nodeNames
  )

  const submit = form.handleSubmit((values) => {
    onApply({
      accountIds: values.accountIds.map(parseIdString),
      channelKeys: values.channelKeys,
      customerIds: values.customerIds.map(parseIdString),
      models: values.models,
      nodeNames: values.nodeNames,
      page: 1,
      siteIds: values.siteIds.map(parseIdString),
      tokenKeys: values.tokenKeys,
      useGroups: values.useGroups,
    })
  })

  return (
    <FilterPanel
      description={t('statistics.filter.description')}
      hasAdvancedActive={count > 0}
      onApply={() => void submit()}
      onReset={() =>
        form.reset({
          accountIds: [],
          channelKeys: [],
          customerIds: [],
          models: [],
          nodeNames: [],
          siteIds: [],
          tokenKeys: [],
          useGroups: [],
        })
      }
      title={t('statistics.filter.title')}
    >
      <form
        className='grid min-w-0 gap-5'
        onSubmit={(event) => {
          event.preventDefault()
          void submit()
        }}
      >
        <FilterGroup
          emptyLabel={t('statistics.filter.noOptions')}
          label={t('statistics.filter.sites')}
          onChange={(values) => setValues('siteIds', values)}
          options={siteOptions}
          values={siteIds}
        />
        {supports(scope, 'customerIds') && (
          <FilterGroup
            emptyLabel={t('statistics.filter.noOptions')}
            label={t('statistics.filter.customers')}
            onChange={(values) => setValues('customerIds', values)}
            options={customerOptions}
            values={customerIds}
          />
        )}
        {supports(scope, 'accountIds') && (
          <FilterGroup
            emptyLabel={t('statistics.filter.noOptions')}
            label={t('statistics.filter.accounts')}
            onChange={(values) => setValues('accountIds', values)}
            options={accountOptions}
            values={accountIds}
          />
        )}
        {(['model', 'channel', 'group', 'token', 'node'] as const).includes(
          scope as 'model' | 'channel' | 'group' | 'token' | 'node'
        ) && (
          <label className='grid gap-1 text-sm'>
            <span>{t('statistics.filter.optionSearch')}</span>
            <Input
              onChange={(event) => setKeyword(event.target.value)}
              placeholder={t('statistics.filter.optionSearchPlaceholder')}
              value={keyword}
            />
          </label>
        )}
        {scope === 'model' && (
          <FilterGroup
            emptyLabel={t('statistics.filter.noOptions')}
            label={t('statistics.filter.models')}
            onChange={(values) => setValues('models', values)}
            options={modelOptions}
            values={models}
          />
        )}
        {scope === 'channel' && (
          <FilterGroup
            emptyLabel={t('statistics.filter.noOptions')}
            label={t('statistics.filter.channels')}
            onChange={(values) => setValues('channelKeys', values)}
            options={channelOptions}
            values={channelKeys}
          />
        )}
        {scope === 'group' && (
          <FilterGroup
            emptyLabel={t('statistics.filter.noOptions')}
            label={t('statistics.filter.groups')}
            onChange={(values) => setValues('useGroups', values)}
            options={groupOptions}
            values={useGroups}
          />
        )}
        {scope === 'token' && (
          <FilterGroup
            emptyLabel={t('statistics.filter.noOptions')}
            label={t('statistics.filter.tokens')}
            onChange={(values) => setValues('tokenKeys', values)}
            options={tokenOptions}
            values={tokenKeys}
          />
        )}
        {scope === 'node' && (
          <FilterGroup
            emptyLabel={t('statistics.filter.noOptions')}
            label={t('statistics.filter.nodes')}
            onChange={(values) => setValues('nodeNames', values)}
            options={nodeOptions}
            values={nodeNames}
          />
        )}
      </form>
    </FilterPanel>
  )
}
