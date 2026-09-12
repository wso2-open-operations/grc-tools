import { describe, expect, test } from "vitest";
import { detectChangingSteps } from "./detectChangingSteps";

// Shorthand: "3:deletion" reads better in a failure than a nested object.
const flags = (subtasks: string[]) =>
  detectChangingSteps(subtasks).map((f) => `${f.stepNumber}:${f.group}`);

describe("detectChangingSteps", () => {
  test("a step that is only a changing verb is flagged", () => {
    expect(flags(["delete"])).toEqual(["1:deletion"]);
    expect(flags(["update"])).toEqual(["1:update"]);
  });

  test("a step with no list marker space is still flagged", () => {
    // The prompt parser needs a space after "1." to split a list item, so a
    // prompt typed as "1.delete" arrives here as one step still carrying its
    // marker. Matching anywhere is what makes that work.
    expect(flags(["1.delete"])).toEqual(["1:deletion"]);
    expect(flags(["2)remove the bucket"])).toEqual(["1:deletion"]);
  });

  test("a changing verb at the end of a step is flagged", () => {
    expect(flags(["find chalaka123 key vaults and delete"])).toEqual(["1:deletion"]);
    expect(flags(["go to the resource group, then remove it"])).toEqual(["1:deletion"]);
  });

  test("a changing verb after a polite opening is flagged", () => {
    expect(flags(["Please delete the test account"])).toEqual(["1:deletion"]);
  });

  test("every word form of a verb is flagged", () => {
    expect(flags(["delete it"])).toEqual(["1:deletion"]);
    expect(flags(["deletes it"])).toEqual(["1:deletion"]);
    expect(flags(["deleted it"])).toEqual(["1:deletion"]);
    expect(flags(["deleting it"])).toEqual(["1:deletion"]);
  });

  test("case does not matter", () => {
    expect(flags(["DELETE the test user"])).toEqual(["1:deletion"]);
    expect(flags(["Delete the test user"])).toEqual(["1:deletion"]);
  });

  test("every added verb is matched", () => {
    const cases: Array<[string, string]> = [
      ["destroy the vm", "deletion"], ["terminate the instance", "deletion"],
      ["purge the vault", "deletion"], ["wipe the disk", "deletion"],
      ["erase the logs", "deletion"], ["drop the table", "deletion"],
      ["deallocate the vm", "deletion"], ["detach the disk", "deletion"],
      ["uninstall the agent", "deletion"],
      ["modify the policy", "update"], ["replace the cert", "update"],
      ["overwrite the config", "update"], ["reset the password", "update"],
      ["patch the host", "update"],
      ["provision a cluster", "creation"], ["deploy the app", "creation"],
      ["enable the flag", "access change"], ["assign the role", "access change"],
      ["unassign the role", "access change"], ["rotate the key", "access change"],
      ["stop the vm", "power change"], ["restart the service", "power change"],
      ["reboot the host", "power change"], ["shutdown the cluster", "power change"],
      ["kill the process", "power change"],
    ];
    for (const [step, group] of cases) {
      expect(flags([step])).toEqual([`1:${group}`]);
    }
  });

  test("the awkward word forms are spelled the way people type them", () => {
    // These are the ones a naive generator gets wrong: it would produce
    // "droping", "modifyed" and "detachs", and the real words would sail
    // straight past the warning.
    for (const step of ["dropped the table", "dropping the table", "drops the table"]) {
      expect(flags([step])).toEqual(["1:deletion"]);
    }
    for (const step of ["modified the policy", "modifies the policy", "modifying the policy"]) {
      expect(flags([step])).toEqual(["1:update"]);
    }
    for (const step of ["detaches the disk", "detached the disk"]) {
      expect(flags([step])).toEqual(["1:deletion"]);
    }
    for (const step of ["stopped the vm", "stopping the vm"]) {
      expect(flags([step])).toEqual(["1:power change"]);
    }
    expect(flags(["destroyed the vault"])).toEqual(["1:deletion"]);
    expect(flags(["deployed the app"])).toEqual(["1:creation"]);
  });

  test("the three rejected words stay silent", () => {
    // "change", "set" and "add" turn up in ordinary capture prompts far too
    // often to be worth a tick. Kept as a test so nobody adds them back
    // without meeting this.
    expect(flags(["change the filter to last 30 days"])).toEqual([]);
    expect(flags(["set the date range to this month"])).toEqual([]);
    expect(flags(["add a column and screenshot the table"])).toEqual([]);
    expect(flags(["edit the search box, then screenshot"])).toEqual([]);
  });

  test("one case per verb group", () => {
    expect(flags(["delete the vault"])).toEqual(["1:deletion"]);
    expect(flags(["remove the vault"])).toEqual(["1:deletion"]);
    expect(flags(["update the policy"])).toEqual(["1:update"]);
    expect(flags(["rename the group"])).toEqual(["1:rename"]);
    expect(flags(["create a new bucket"])).toEqual(["1:creation"]);
    expect(flags(["grant reader to the group"])).toEqual(["1:access change"]);
    expect(flags(["revoke the key"])).toEqual(["1:access change"]);
    expect(flags(["disable the protection setting"])).toEqual(["1:access change"]);
  });

  test("an ordinary capture prompt is silent", () => {
    expect(flags(["Go to Key Vault X and screenshot the access policy"])).toEqual([]);
    expect(flags(["Go to S3, find bucket cloudcare-k8s, screenshot the objects list"])).toEqual([]);
    expect(flags(["find India cricket"])).toEqual([]);
  });

  test("the application's own example prompts are silent", () => {
    // Both come from the help text the Agent Runner page shows. If either
    // ever starts warning, the page is teaching the banner to be ignored.
    expect(
      flags(['Go to Key Vaults, filter by label "env:prod"', "EACH-PAGE: Screenshot page {page} of the filtered results"])
    ).toEqual([]);
    expect(
      flags(["PDF: Open https://github.com/org/repo/issues/123, expand all comments, then export as PDF"])
    ).toEqual([]);
  });

  test("a capture prompt that merely names a delete setting IS flagged, on purpose", () => {
    // This is the deliberate cost of matching everywhere. The Engineer ticks
    // the box once. Missing a real deletion costs a resource; this costs a
    // click. See the module comment.
    expect(flags(["Screenshot the delete protection setting"])).toEqual(["1:deletion"]);
    expect(flags(["Screenshot the update history for this resource"])).toEqual(["1:update"]);
  });

  test("a verb inside a longer word is not flagged", () => {
    expect(flags(["undelete the row"])).toEqual([]);
    expect(flags(["screenshot the deletion policy page"])).toEqual([]);
    expect(flags(["open the creation date column"])).toEqual([]);
  });

  test("several flagged steps come back with the right numbers", () => {
    expect(
      flags([
        "Go to S3, screenshot objects",
        "Delete the old bucket",
        "Screenshot the results",
        "rename the group",
      ])
    ).toEqual(["2:deletion", "4:rename"]);
  });

  test("nothing flagged returns an empty result", () => {
    expect(flags(["screenshot the page", "export it as a PDF"])).toEqual([]);
  });

  test("no steps at all returns an empty result", () => {
    expect(flags([])).toEqual([]);
  });

  test("a step matching two groups reports the first in table order", () => {
    expect(flags(["delete and rename the group"])).toEqual(["1:deletion"]);
  });

  test("a multi line step is searched in full, not just its first line", () => {
    expect(flags(["Go to the vault\nthen delete it"])).toEqual(["1:deletion"]);
  });
});
