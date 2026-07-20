import { z } from 'zod'

import {
  displayNameSchema,
  passwordSchema,
  usernameSchema,
} from '../auth/schema'

export const platformUserSearchSchema = z.object({
  filter: z.string().max(128).optional().catch(undefined),
  page: z.coerce.number().int().min(1).optional().catch(undefined),
  pageSize: z.coerce.number().int().min(1).max(100).optional().catch(undefined),
  role: z.enum(['admin', 'viewer']).optional().catch(undefined),
  status: z
    .union([z.literal(1), z.literal(2)])
    .optional()
    .catch(undefined),
})

export const createPlatformUserSchema = z
  .object({
    confirmPassword: z.string().min(1, { message: 'Confirm password' }),
    displayName: displayNameSchema,
    password: passwordSchema,
    role: z.enum(['admin', 'viewer']),
    username: usernameSchema,
  })
  .superRefine((value, context) => {
    if (value.password !== value.confirmPassword) {
      context.addIssue({
        code: 'custom',
        message: 'Passwords do not match',
        path: ['confirmPassword'],
      })
    }
  })

export const editPlatformUserSchema = z.object({
  displayName: displayNameSchema,
  role: z.enum(['admin', 'viewer']),
  username: usernameSchema,
})

export const resetPlatformUserPasswordSchema = z
  .object({
    confirmPassword: z.string().min(1, { message: 'Confirm password' }),
    password: passwordSchema,
  })
  .superRefine((value, context) => {
    if (value.password !== value.confirmPassword) {
      context.addIssue({
        code: 'custom',
        message: 'Passwords do not match',
        path: ['confirmPassword'],
      })
    }
  })

export type CreatePlatformUserFormValues = z.infer<
  typeof createPlatformUserSchema
>
export type EditPlatformUserFormValues = z.infer<typeof editPlatformUserSchema>
export type ResetPlatformUserPasswordFormValues = z.infer<
  typeof resetPlatformUserPasswordSchema
>
