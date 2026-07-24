import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { FacetedFilter } from '@/components/data/faceted-filter'
import { FilterPanel } from '@/components/data/filter-panel'
import { SelectControl as Select } from '@/components/ui/select-control'
import type { SiteListItem } from '@/features/sites/types'
import { isIdString } from '@/lib/api-types'
import { hasFilterChanges } from '@/lib/filter-state'

import { alertLevels, alertRuleCategories } from '../constants'
import { alertRuleCategoryText } from '../rule-text'
import type { AlertSearch } from '../types'
import { alertLevelText } from './alert-ui'

export type AlertRuleFilterValue = Pick<
  AlertSearch,
  | 'ruleCategory'
  | 'ruleEnabled'
  | 'ruleInherited'
  | 'ruleLevel'
  | 'ruleSiteId'
  | 'scope'
>

function filterValue(value: AlertRuleFilterValue): AlertRuleFilterValue {
  return {
    ...value,
    ruleCategory: [...value.ruleCategory],
    ruleLevel: [...value.ruleLevel],
  }
}

const resetValue: AlertRuleFilterValue = {
  ruleCategory: [],
  ruleEnabled: undefined,
  ruleInherited: undefined,
  ruleLevel: [],
  ruleSiteId: undefined,
  scope: 'global',
}

function optionalBoolean(value: string): boolean | undefined {
  if (value === 'true') return true
  if (value === 'false') return false
  return undefined
}

export function AlertRuleFilters({
  onApply,
  sites,
  value,
}: {
  onApply: (value: AlertRuleFilterValue) => void
  sites: SiteListItem[]
  value: AlertRuleFilterValue
}) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState(() => filterValue(value))

  useEffect(() => setDraft(filterValue(value)), [value])

  const applyImmediately = (next: AlertRuleFilterValue) => {
    setDraft(next)
    onApply(next)
  }

  return (
    <FilterPanel
      description={t('alerts.rules.filtersDescription')}
      hasActiveFilters={hasFilterChanges(draft, resetValue, [
        'ruleCategory',
        'ruleEnabled',
        'ruleInherited',
        'ruleLevel',
        'ruleSiteId',
        'scope',
      ])}
      onReset={() => {
        const reset = filterValue(resetValue)
        applyImmediately(reset)
      }}
      title={t('alerts.rules.filters')}
    >
      <div className='flex flex-wrap items-end gap-2'>
        <label className='grid gap-1.5 text-sm'>
          <span className='font-medium'>{t('alerts.rules.scope')}</span>
          <Select
            onChange={(event) =>
              applyImmediately({
                ...draft,
                ruleInherited:
                  event.target.value === 'site'
                    ? draft.ruleInherited
                    : undefined,
                ruleSiteId:
                  event.target.value === 'site' ? draft.ruleSiteId : undefined,
                scope: event.target.value as AlertSearch['scope'],
              })
            }
            value={draft.scope}
          >
            <option value='global'>{t('alerts.rules.scope.global')}</option>
            <option value='site'>{t('alerts.rules.scope.site')}</option>
          </Select>
        </label>
        {draft.scope === 'site' && (
          <label className='grid gap-1.5 text-sm'>
            <span className='font-medium'>{t('alerts.rules.site')}</span>
            <Select
              onChange={(event) =>
                applyImmediately({
                  ...draft,
                  ruleSiteId: isIdString(event.target.value)
                    ? event.target.value
                    : undefined,
                })
              }
              value={draft.ruleSiteId ?? ''}
            >
              <option value=''>{t('alerts.rules.selectSite')}</option>
              {sites.map((site) => (
                <option key={site.id} value={site.id}>
                  {site.name}
                </option>
              ))}
            </Select>
          </label>
        )}
        <FacetedFilter
          clearLabel={t('common.clearFilters')}
          onChange={(category) =>
            applyImmediately({
              ...draft,
              ruleCategory: alertRuleCategories.includes(
                category as (typeof alertRuleCategories)[number]
              )
                ? [category as (typeof alertRuleCategories)[number]]
                : [],
            })
          }
          options={alertRuleCategories.map((category) => ({
            label: alertRuleCategoryText(t, category),
            value: category,
          }))}
          title={t('alerts.rules.category')}
          value={draft.ruleCategory[0] ?? ''}
        />
        <FacetedFilter
          clearLabel={t('common.clearFilters')}
          onChange={(level) =>
            applyImmediately({
              ...draft,
              ruleLevel: alertLevels.includes(
                level as (typeof alertLevels)[number]
              )
                ? [level as (typeof alertLevels)[number]]
                : [],
            })
          }
          options={alertLevels.map((level) => ({
            label: alertLevelText(t, level),
            value: level,
          }))}
          title={t('alerts.rules.level')}
          value={draft.ruleLevel[0] ?? ''}
        />
        <FacetedFilter
          clearLabel={t('common.clearFilters')}
          onChange={(enabled) =>
            applyImmediately({
              ...draft,
              ruleEnabled: optionalBoolean(enabled),
            })
          }
          options={[
            { label: t('alerts.rules.enabledValue'), value: 'true' },
            { label: t('alerts.rules.disabledValue'), value: 'false' },
          ]}
          title={t('alerts.rules.enabled')}
          value={draft.ruleEnabled == null ? '' : String(draft.ruleEnabled)}
        />
        {draft.scope === 'site' && (
          <FacetedFilter
            clearLabel={t('common.clearFilters')}
            onChange={(inherited) =>
              applyImmediately({
                ...draft,
                ruleInherited: optionalBoolean(inherited),
              })
            }
            options={[
              { label: t('alerts.rules.inherited'), value: 'true' },
              { label: t('alerts.rules.overridden'), value: 'false' },
            ]}
            title={t('alerts.rules.inheritance')}
            value={
              draft.ruleInherited == null ? '' : String(draft.ruleInherited)
            }
          />
        )}
      </div>
    </FilterPanel>
  )
}
