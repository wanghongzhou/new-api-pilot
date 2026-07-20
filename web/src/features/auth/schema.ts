import { z } from 'zod'

const textEncoder = new TextEncoder()

export const usernameSchema = z
  .string()
  .trim()
  .toLowerCase()
  .min(3, { message: 'Username must contain 3 to 64 characters' })
  .max(64, { message: 'Username must contain 3 to 64 characters' })
  .regex(/^[a-z0-9._-]+$/, {
    message: 'Username contains unsupported characters',
  })

export const passwordSchema = z
  .string()
  .min(1, { message: 'Password is required' })
  .superRefine((value, context) => {
    if (Array.from(value).length < 8) {
      context.addIssue({
        code: 'custom',
        message: 'Password must contain at least 8 Unicode characters',
      })
    }
    if (textEncoder.encode(value).byteLength > 72) {
      context.addIssue({
        code: 'custom',
        message: 'Password must not exceed 72 UTF-8 bytes',
      })
    }
  })

export const displayNameSchema = z
  .string()
  .trim()
  .min(1, { message: 'Display name is required' })
  .superRefine((value, context) => {
    if (Array.from(value).length > 128) {
      context.addIssue({
        code: 'custom',
        message: 'Display name must not exceed 128 Unicode characters',
      })
    }
  })

export const loginSchema = z.object({
  username: usernameSchema,
  password: z.string().min(1, { message: 'Password is required' }),
})

export const changePasswordSchema = z
  .object({
    originalPassword: z.string().min(1, { message: 'Password is required' }),
    newPassword: passwordSchema,
    confirmPassword: z.string().min(1, { message: 'Confirm password' }),
  })
  .superRefine((value, context) => {
    if (value.newPassword !== value.confirmPassword) {
      context.addIssue({
        code: 'custom',
        message: 'Passwords do not match',
        path: ['confirmPassword'],
      })
    }
    if (value.originalPassword === value.newPassword) {
      context.addIssue({
        code: 'custom',
        message: 'New password must be different from the current password',
        path: ['newPassword'],
      })
    }
  })

export type LoginFormValues = z.infer<typeof loginSchema>
export type ChangePasswordFormValues = z.infer<typeof changePasswordSchema>
