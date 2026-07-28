export class SessionVerificationError extends Error {
  readonly originalError: Error

  constructor(error: Error) {
    super('Session verification failed')
    this.name = 'SessionVerificationError'
    this.originalError = error
  }
}
