// Kept in step with the server's own caps in
// backend/app/api/routes/evidence.py (MAX_FILES_PER_SUBMISSION,
// MAX_TOTAL_UPLOAD_BYTES). The server is the real limit; this only lets the
// Submit page say so before a doomed upload ever starts.
export const MAX_SUBMISSION_FILES = 4;
export const MAX_SUBMISSION_TOTAL_BYTES = 20 * 1024 * 1024; // 20 MB

export type ValidateSubmissionFilesResult =
  | { valid: true }
  | { valid: false; message: string };

/**
 * Checks a chosen file list against the two rules the server also enforces:
 * at least one file and no more than MAX_SUBMISSION_FILES, and no more than
 * MAX_SUBMISSION_TOTAL_BYTES combined. Returns why the selection is invalid
 * so the page can show it as is, rather than the page working out its own
 * wording.
 *
 * Plain data in, plain data out — no React, no knowledge of how the page
 * renders the file list.
 */
export function validateSubmissionFiles(files: File[]): ValidateSubmissionFilesResult {
  if (files.length === 0) {
    return { valid: false, message: "Please choose a file to upload." };
  }

  if (files.length > MAX_SUBMISSION_FILES) {
    return {
      valid: false,
      message: `Please choose ${MAX_SUBMISSION_FILES} files or fewer.`,
    };
  }

  const totalBytes = files.reduce((sum, file) => sum + file.size, 0);
  if (totalBytes > MAX_SUBMISSION_TOTAL_BYTES) {
    return {
      valid: false,
      message: `These files add up to more than ${MAX_SUBMISSION_TOTAL_BYTES / (1024 * 1024)} MB combined. Please choose fewer or smaller files.`,
    };
  }

  return { valid: true };
}
