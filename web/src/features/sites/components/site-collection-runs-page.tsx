import { ArrowLeft01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { DetailBackLink } from '@/components/layout/detail-back-link'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { isIdString, parseIdString } from '@/lib/api-types'
import { useAuthStore } from '@/stores/auth-store'

import { getSite } from '../api'
import { siteKeys } from '../query-keys'
import type {
  CollectionRunItem,
  CollectionRunWindowItem,
  CollectionTaskType,
  FastCollectionTaskType,
  FastTaskHistoryItem,
} from '../types'
import { CollectionRunsPanel } from './collection-runs-panel'
import { FastTaskHistoryPanel } from './fast-task-history-panel'

export interface SiteCollectionRunsSearch {
  fastPage: number
  fastStatus?: FastTaskHistoryItem['status']
  fastTaskType: FastCollectionTaskType
  runId?: string
  runPage: number
  runStatus?: CollectionRunItem['status']
  runTaskType?: CollectionTaskType
  tab: 'fast' | 'runs'
  windowPage: number
  windowStatus?: CollectionRunWindowItem['status']
}

export function SiteCollectionRunsPage({
  onSearchChange,
  search,
  siteId,
}: {
  onSearchChange: (changes: Partial<SiteCollectionRunsSearch>) => void
  search: SiteCollectionRunsSearch
  siteId: string
}) {
  const { t } = useTranslation()
  const isAdmin = useAuthStore((state) => state.user?.role === 'admin')
  const validSiteId = isIdString(siteId)
  const siteQuery = useQuery({
    enabled: validSiteId,
    queryFn: () => getSite(parseIdString(siteId)),
    queryKey: siteKeys.detail(siteId),
    staleTime: 30_000,
  })

  return (
    <SectionPageLayout
      description={t('collection.description')}
      title={
        siteQuery.data
          ? t('site.collectionPage.titleWithSite', {
              site: siteQuery.data.name,
            })
          : t('site.actions.collectionRuns')
      }
    >
      <div className='grid min-w-0 gap-6'>
        <DetailBackLink
          render={<Link params={{ siteId }} to='/sites/$siteId' />}
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
          {t('site.collectionPage.backToDetail')}
        </DetailBackLink>
        <Tabs
          onValueChange={(tab) =>
            onSearchChange({
              runId: tab === 'fast' ? undefined : search.runId,
              tab: tab as SiteCollectionRunsSearch['tab'],
            })
          }
          value={search.tab}
        >
          <TabsList aria-label={t('collection.tabs.label')}>
            <TabsTrigger value='runs'>{t('collection.tabs.runs')}</TabsTrigger>
            <TabsTrigger value='fast'>{t('collection.tabs.fast')}</TabsTrigger>
          </TabsList>
          <TabsContent value='runs'>
            {search.tab === 'runs' && (
              <CollectionRunsPanel
                isAdmin={Boolean(isAdmin)}
                onSearchChange={onSearchChange}
                search={search}
                siteId={siteId}
              />
            )}
          </TabsContent>
          <TabsContent value='fast'>
            {search.tab === 'fast' && (
              <FastTaskHistoryPanel
                onSearchChange={onSearchChange}
                search={search}
                siteId={siteId}
              />
            )}
          </TabsContent>
        </Tabs>
      </div>
    </SectionPageLayout>
  )
}
