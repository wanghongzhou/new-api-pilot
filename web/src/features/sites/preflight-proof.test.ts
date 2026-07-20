import { describe, expect, test } from 'bun:test'

import {
  preflightProofInvalidReason,
  publicUrlParts,
  type SitePreflightProof,
} from './preflight-proof'

const proof = {
  configVersion: 7,
  result: {
    expires_at: 200,
    normalized_base_url: 'https://api.example.com/v1',
  },
} as SitePreflightProof

describe('site base URL preflight proof', () => {
  test('accepts an equivalent canonical candidate while proof is current', () => {
    expect(
      preflightProofInvalidReason(proof, 'https://API.EXAMPLE.com/v1/', 7, 199)
    ).toBeNull()
  })

  test('invalidates URL, config version, and expiration changes', () => {
    expect(
      preflightProofInvalidReason(proof, 'https://api.example.com/v2', 7, 100)
    ).toBe('candidate_changed')
    expect(
      preflightProofInvalidReason(proof, 'https://api.example.com/v1', 8, 100)
    ).toBe('config_changed')
    expect(
      preflightProofInvalidReason(proof, 'https://api.example.com/v1', 7, 200)
    ).toBe('expired')
  })

  test('separates origin and path for explicit identity comparison', () => {
    expect(publicUrlParts('https://api.example.com/root/v1')).toEqual({
      origin: 'https://api.example.com',
      path: '/root/v1',
    })
  })
})
