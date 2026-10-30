// A job the user canceled. Kept apart from the API client so code that
// checks for it keeps working when tests replace the client with mocks.
export class JobCanceledError extends Error {
  constructor() {
    super("Job canceled.");
    this.name = "JobCanceledError";
  }
}

export function isJobCanceled(error: unknown) {
  return error instanceof Error && error.name === "JobCanceledError";
}
