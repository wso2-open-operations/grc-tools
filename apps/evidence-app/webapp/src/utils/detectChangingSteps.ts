// The verb groups a step can fall into. Kept as a union rather than a bare
// string so a typo in a call site is a compile error, not a silent no-match.
export type ChangingVerbGroup =
  | "deletion"
  | "update"
  | "rename"
  | "creation"
  | "access change"
  | "power change";

type ChangingVerbGroupRow = {
  group: ChangingVerbGroup;
  // Present tense stems. Their other forms are generated below, so a row
  // never has to list "deletes", "deleted" and "deleting" by hand.
  stems: string[];
};

// One row per group of changing verbs the Agent Runner should pause on.
//
// These are matched ANYWHERE in a step, not only as its first word — see
// chala2001/grc-tools#136. An earlier version read the leading verb only, on
// the reasoning that "screenshot the delete protection setting" is an
// ordinary Evidence prompt and should stay silent. That reasoning holds if
// the warning BLOCKS. It does not: the warning is a banner and a tick box,
// and the Engineer always gets through. So a false positive costs one tick
// and a miss costs a deleted resource, which is a lopsided enough trade that
// matching everywhere is the safer rule. It is also a rule an Engineer can
// predict without reading this file, which is worth a great deal on its own.
//
// Matching everywhere is what makes the awkward phrasings work: "1.delete"
// with no space after the marker, "find the key vault and delete", "please
// delete the account", "deleting the account". Each of those slipped past
// the leading word rule.
const CHANGING_VERB_GROUPS: ChangingVerbGroupRow[] = [
  {
    group: "deletion",
    // Outright destruction, in all the words a console actually uses for
    // it. "drop" is here for databases and tables; "deallocate", "detach"
    // and "uninstall" take something away without the word delete ever
    // appearing.
    stems: [
      "delete", "remove", "destroy", "terminate", "purge", "wipe", "erase",
      "drop", "deallocate", "detach", "uninstall",
    ],
  },
  {
    group: "update",
    // Changing something in place. "change" and "set" are deliberately
    // absent: "change the filter" and "set the date range" are ordinary
    // capture instructions, and now that matching is everywhere those two
    // would fire on a large share of perfectly safe prompts. "edit" is left
    // out for the same reason, though it is the closest call of the three.
    stems: ["update", "modify", "replace", "overwrite", "reset", "patch"],
  },
  {
    group: "rename",
    stems: ["rename"],
  },
  {
    group: "creation",
    // "add" is deliberately absent: it is used loosely for all sorts of non
    // mutating things ("add a bookmark", "add to the list"), and now that
    // matching is everywhere it would fire on far too much.
    stems: ["create", "provision", "deploy"],
  },
  {
    group: "access change",
    // Handing out, taking away, switching on or switching off. "enable" is
    // included now: turning something on is a real change to a console even
    // when what it turns on is read only, and the cost of being wrong is a
    // tick.
    stems: ["grant", "revoke", "disable", "enable", "assign", "unassign", "rotate"],
  },
  {
    group: "power change",
    // Changes nothing stored, but takes a live system down or bounces it,
    // which is the kind of thing nobody wants to discover was in a prompt
    // by accident.
    stems: ["stop", "restart", "reboot", "shutdown", "kill"],
  },
];

// Present tense, third person, past and continuous, spelled the way English
// actually spells them. Three ordinary rules cover every stem in the table:
// a stem ending in "e" drops it before "ing" and takes a bare "d"
// ("deleting", "deleted"); a stem ending in a consonant then "y" becomes
// "ied" and "ies" ("modified", "modifies"); and a short stem ending
// consonant, vowel, consonant doubles that last letter ("dropped",
// "stopping"). Without those, "modifyed" and "droping" would be generated
// and the words anyone actually types would never match.
function wordForms(stem: string): string[] {
  // "modify" gives "modifies", "patch" and "detach" give "patches" and
  // "detaches", everything else just takes an "s".
  const thirdPerson = /[^aeiou]y$/.test(stem)
    ? `${stem.slice(0, -1)}ies`
    : /(?:ch|sh|s|x|z)$/.test(stem)
      ? `${stem}es`
      : `${stem}s`;

  const forms = [stem, thirdPerson];

  if (stem.endsWith("e")) {
    forms.push(`${stem}d`, `${stem.slice(0, -1)}ing`);
    return forms;
  }

  if (/[^aeiou]y$/.test(stem)) {
    forms.push(`${stem.slice(0, -1)}ied`, `${stem}ing`);
    return forms;
  }

  // Consonant, vowel, consonant at the end, with the last letter not one of
  // w, x or y, which never double.
  const doubled = /[^aeiou][aeiou][^aeiouwxy]$/.test(stem) ? stem + stem.slice(-1) : stem;
  forms.push(`${doubled}ed`, `${doubled}ing`);
  return forms;
}

// One regex per group, built once at module load. \b on both sides keeps a
// form from matching inside a longer word, so "undelete" and "deletion" do
// not trip the deletion row.
const GROUP_PATTERNS: { group: ChangingVerbGroup; regex: RegExp }[] = CHANGING_VERB_GROUPS.map(
  (row) => ({
    group: row.group,
    regex: new RegExp(`\\b(?:${row.stems.flatMap(wordForms).join("|")})\\b`, "i"),
  })
);

export type ChangingStepFlag = {
  // 1 based, matching the numbers the page already shows next to the
  // parsed task list — see chala2001/grc-tools#140.
  stepNumber: number;
  group: ChangingVerbGroup;
};

/**
 * Looks at each already parsed subtask and says which ones mention changing
 * something rather than only looking at it. Takes the same subtask list the
 * Agent Runner page already parses out of the prompt and renders as its
 * numbered list, so a step number named here is always a step number that
 * exists on screen — see chala2001/grc-tools#140.
 *
 * A step is flagged when any form of a changing verb appears anywhere in it.
 * That deliberately includes ordinary capture prompts that merely name a
 * delete policy or an update history: they raise the banner, the Engineer
 * ticks the box once, and nothing is ever refused. See the comment on
 * CHANGING_VERB_GROUPS above for why that trade is the right way round.
 *
 * A step matching more than one group reports the first group in table
 * order, because the banner only needs to say what kind of change it saw,
 * not enumerate every one.
 *
 * Plain data in, plain data out — no React, no knowledge of how any of this
 * is drawn.
 */
export function detectChangingSteps(subtasks: string[]): ChangingStepFlag[] {
  const flagged: ChangingStepFlag[] = [];
  subtasks.forEach((step, index) => {
    const hit = GROUP_PATTERNS.find((p) => p.regex.test(step));
    if (hit) {
      flagged.push({ stepNumber: index + 1, group: hit.group });
    }
  });
  return flagged;
}
