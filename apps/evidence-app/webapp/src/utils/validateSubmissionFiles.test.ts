import { describe, expect, test } from "vitest";
import {
  MAX_SUBMISSION_FILES,
  MAX_SUBMISSION_TOTAL_BYTES,
  validateSubmissionFiles,
} from "./validateSubmissionFiles";

function fileOfSize(name: string, bytes: number): File {
  return new File([new Uint8Array(bytes)], name);
}

describe("validateSubmissionFiles", () => {
  test("no files at all is invalid", () => {
    expect(validateSubmissionFiles([])).toEqual({
      valid: false,
      message: "Please choose a file to upload.",
    });
  });

  test("one small file is valid", () => {
    expect(validateSubmissionFiles([fileOfSize("a.png", 100)])).toEqual({ valid: true });
  });

  test("exactly the file count cap is valid", () => {
    const files = Array.from({ length: MAX_SUBMISSION_FILES }, (_, i) =>
      fileOfSize(`f${i}.png`, 100)
    );
    expect(validateSubmissionFiles(files)).toEqual({ valid: true });
  });

  test("one more than the file count cap is invalid", () => {
    const files = Array.from({ length: MAX_SUBMISSION_FILES + 1 }, (_, i) =>
      fileOfSize(`f${i}.png`, 100)
    );
    expect(validateSubmissionFiles(files)).toEqual({
      valid: false,
      message: `Please choose ${MAX_SUBMISSION_FILES} files or fewer.`,
    });
  });

  test("exactly the total size cap is valid", () => {
    expect(validateSubmissionFiles([fileOfSize("big.png", MAX_SUBMISSION_TOTAL_BYTES)])).toEqual({
      valid: true,
    });
  });

  test("one byte over the total size cap is invalid", () => {
    const result = validateSubmissionFiles([fileOfSize("big.png", MAX_SUBMISSION_TOTAL_BYTES + 1)]);
    expect(result.valid).toBe(false);
    expect((result as { message: string }).message).toContain("more than 20 MB combined");
  });

  test("several files that add up over the cap are invalid even though none is over it alone", () => {
    const each = MAX_SUBMISSION_TOTAL_BYTES / 2 + 1;
    const result = validateSubmissionFiles([fileOfSize("a.png", each), fileOfSize("b.png", each)]);
    expect(result.valid).toBe(false);
  });

  test("the file count check wins when a selection breaks both rules", () => {
    const files = Array.from({ length: MAX_SUBMISSION_FILES + 1 }, (_, i) =>
      fileOfSize(`f${i}.png`, MAX_SUBMISSION_TOTAL_BYTES)
    );
    expect(validateSubmissionFiles(files)).toEqual({
      valid: false,
      message: `Please choose ${MAX_SUBMISSION_FILES} files or fewer.`,
    });
  });
});
