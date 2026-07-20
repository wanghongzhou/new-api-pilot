import { normalizeBaseUrl } from './schema'
import type { SiteBaseUrlPreflightResult } from './types'

export interface SitePreflightProof {
  configVersion: number
  result: SiteBaseUrlPreflightResult
}

export type PreflightProofInvalidReason =
  | 'candidate_changed'
  | 'config_changed'
  | 'expired'

export function publicUrlParts(value: string): {
  origin: string
  path: string
} {
  const url = new URL(value)
  return { origin: url.origin, path: url.pathname || '/' }
}

export function preflightProofInvalidReason(
  proof: SitePreflightProof,
  candidateBaseUrl: string,
  currentConfigVersion: number,
  nowUnix: number
): PreflightProofInvalidReason | null {
  let normalizedCandidate: string
  try {
    normalizedCandidate = normalizeBaseUrl(candidateBaseUrl)
  } catch {
    return 'candidate_changed'
  }
  if (normalizedCandidate !== proof.result.normalized_base_url) {
    return 'candidate_changed'
  }
  if (currentConfigVersion !== proof.configVersion) return 'config_changed'
  if (nowUnix >= proof.result.expires_at) return 'expired'
  return null
}
