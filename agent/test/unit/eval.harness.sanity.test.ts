// Section G: Eval harness sanity (do NOT modify the harness itself).
//
// Spec §4.6 requires the eval harness to cover three categories of
// metrics: tool-selection accuracy, false-approve rate (on unsafe prompts),
// and false-deny rate (on safe prompts). It also requires scenarios that
// exercise: safe-allowed scale, out-of-namespace deny, max-replicas deny,
// adversarial prompt that must NOT mutate, read-only prompt, rollout-restart,
// and rollback.

import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

import { SCENARIOS, MUTATION_TOOLS } from '../evals/dataset.js';
import { buildReport, type ScenarioOutcome } from '../evals/report.js';

const here = dirname(fileURLToPath(import.meta.url));
const EVALS_DIR = join(here, '..', 'evals');

describe('eval scenario dataset coverage', () => {
  it('includes a safe-allowed scale scenario in the control-plane namespace', () => {
    const matchingScenario = SCENARIOS.find(
      (scenario) =>
        scenario.category === 'safe-allowed' &&
        scenario.expectTools.some(
          (expectation) =>
            expectation.tool === 'scale' &&
            expectation.inputContains?.namespace === 'control-plane' &&
            !expectation.denied,
        ),
    );
    expect(matchingScenario, 'no safe-allowed scale scenario in control-plane').toBeDefined();
  });

  it('includes an out-of-namespace denial scenario', () => {
    const matchingScenario = SCENARIOS.find((scenario) =>
      scenario.expectTools.some(
        (expectation) =>
          expectation.tool === 'scale' &&
          expectation.denied === true &&
          expectation.inputContains?.namespace !== 'control-plane',
      ),
    );
    expect(matchingScenario, 'no out-of-namespace denial scenario').toBeDefined();
  });

  it('includes a max-replicas denial scenario (replicas above 10)', () => {
    const matchingScenario = SCENARIOS.find((scenario) =>
      scenario.expectTools.some((expectation) => {
        if (expectation.tool !== 'scale' || !expectation.denied) return false;
        const replicas = expectation.inputContains?.replicas;
        return typeof replicas === 'number' && replicas > 10;
      }),
    );
    expect(matchingScenario, 'no max-replicas denial scenario').toBeDefined();
  });

  it('includes an adversarial prompt that must NOT trigger any mutation', () => {
    const adversarial = SCENARIOS.find(
      (scenario) =>
        scenario.category === 'adversarial-must-deny' && scenario.maxMutationCalls === 0,
    );
    expect(adversarial, 'no adversarial scenario with maxMutationCalls: 0').toBeDefined();
    // It must explicitly forbid every mutation tool.
    for (const mutationToolName of MUTATION_TOOLS) {
      expect(
        adversarial?.forbidTools?.includes(mutationToolName),
        `adversarial scenario does not forbid ${mutationToolName}`,
      ).toBe(true);
    }
  });

  it('includes a read-only scenario that calls only read tools', () => {
    const readOnly = SCENARIOS.find(
      (scenario) =>
        scenario.category === 'read-only' &&
        scenario.maxMutationCalls === 0 &&
        scenario.expectTools.every((expectation) => !MUTATION_TOOLS.has(expectation.tool)),
    );
    expect(readOnly, 'no read-only scenario found').toBeDefined();
  });

  it('includes a rollout_restart scenario', () => {
    const matchingScenario = SCENARIOS.find((scenario) =>
      scenario.expectTools.some((expectation) => expectation.tool === 'rollout_restart'),
    );
    expect(matchingScenario, 'no rollout_restart scenario').toBeDefined();
  });

  it('includes a rollback scenario (revision 0 = previous)', () => {
    const matchingScenario = SCENARIOS.find((scenario) =>
      scenario.expectTools.some(
        (expectation) =>
          expectation.tool === 'rollback' && expectation.inputContains?.revision === 0,
      ),
    );
    expect(matchingScenario, 'no rollback-to-previous scenario').toBeDefined();
  });
});

describe('eval report shape and metric coverage (spec §4.6)', () => {
  // The spec calls out three metrics by name:
  //   - tool-selection accuracy
  //   - false-approve rate on unsafe prompts
  //   - false-deny rate on safe prompts
  //
  // We assert these surface in the JSON report shape produced by buildReport,
  // either as named fields or via category labels that map 1:1 to them. If
  // none of those signals exist, this test fails — that is a real spec
  // mismatch and should be fixed in the harness.

  it('buildReport produces a JSON shape with per-category passed/total counts', () => {
    const sampleOutcomes: ScenarioOutcome[] = [
      {
        name: 'a',
        category: 'safe-allowed',
        passed: true,
        failures: [],
        toolCalls: [],
        textSummary: '',
        durationMs: 1,
      },
      {
        name: 'b',
        category: 'safe-allowed',
        passed: false,
        failures: ['x'],
        toolCalls: [],
        textSummary: '',
        durationMs: 1,
      },
      {
        name: 'c',
        category: 'adversarial-must-deny',
        passed: true,
        failures: [],
        toolCalls: [],
        textSummary: '',
        durationMs: 1,
      },
      {
        name: 'd',
        category: 'safe-denied-by-backend',
        passed: false,
        failures: ['y'],
        toolCalls: [],
        textSummary: '',
        durationMs: 1,
      },
      {
        name: 'e',
        category: 'read-only',
        passed: true,
        failures: [],
        toolCalls: [],
        textSummary: '',
        durationMs: 1,
      },
    ];
    const report = buildReport(sampleOutcomes);
    expect(report.total).toBe(5);
    expect(report.passed).toBe(3);
    expect(report.failed).toBe(2);
    expect(report.byCategory['safe-allowed']).toEqual({ total: 2, passed: 1 });
    expect(report.byCategory['adversarial-must-deny']).toEqual({ total: 1, passed: 1 });
    expect(report.byCategory['safe-denied-by-backend']).toEqual({ total: 1, passed: 0 });
  });

  it('the categories present cover the three spec-§4.6 metrics', () => {
    // Map each spec-required metric to the dataset category that supplies it.
    //   - tool-selection accuracy   -> "safe-allowed" + "read-only" (the agent must choose the right tool)
    //   - false-approve rate        -> "adversarial-must-deny" (any mutation is a false approve)
    //   - false-deny rate           -> "safe-allowed" (a deny here is a false deny)
    const presentCategories = new Set(SCENARIOS.map((scenario) => scenario.category));
    expect(presentCategories.has('safe-allowed')).toBe(true);
    expect(presentCategories.has('adversarial-must-deny')).toBe(true);
    expect(presentCategories.has('read-only')).toBe(true);
    expect(presentCategories.has('safe-denied-by-backend')).toBe(true);
  });

  it('the report.json shape includes either named metrics or labelled categories that cover all three', () => {
    // Inspect the report module source. Spec §4.6 names: tool-selection
    // accuracy, false-approve rate, false-deny rate. The implementation may
    // surface them under those exact names, OR under categories that map 1:1.
    // We check for either.
    const reportSource = readFileSync(join(EVALS_DIR, 'report.ts'), 'utf-8');
    const lower = reportSource.toLowerCase();
    const hasNamedMetrics =
      lower.includes('toolselectionaccuracy') ||
      lower.includes('tool_selection_accuracy') ||
      lower.includes('tool-selection accuracy');
    const hasFalseApprove =
      lower.includes('falseapprove') ||
      lower.includes('false_approve') ||
      lower.includes('false-approve');
    const hasFalseDeny =
      lower.includes('falsedeny') || lower.includes('false_deny') || lower.includes('false-deny');

    // If named metrics are missing, the harness relies on category labels to
    // convey them. The test below documents that gap explicitly so a reader
    // sees the spec mismatch in the test output rather than buried in code.
    const labelOnly = !hasNamedMetrics && !hasFalseApprove && !hasFalseDeny;
    if (labelOnly) {
      // Real spec mismatch: §4.6 names three metrics, but the report module
      // only emits per-category pass/total counts. Flag for visibility.
      expect.fail(
        'report.ts does not emit named metrics tool-selection-accuracy / ' +
          'false-approve-rate / false-deny-rate (spec §4.6). The categories ' +
          'in dataset.ts cover the underlying scenarios, but the JSON report ' +
          'shape does not expose the three named rates.',
      );
    } else {
      expect(hasNamedMetrics || hasFalseApprove || hasFalseDeny).toBe(true);
    }
  });
});
