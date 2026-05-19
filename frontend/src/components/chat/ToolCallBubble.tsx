export type GuardrailState = 'pending' | 'allow' | 'deny';

interface ToolCallBubbleProps {
  tool: string;
  input: Record<string, unknown>;
  // Every backend-wrapped tool — both reads and writes — passes through the
  // guardrail enforcer. The badge reflects the enforcer's decision once the
  // matching tool_result arrives; before then it sits in pending.
  guardrailState: GuardrailState;
}

const BADGE_LABEL: Record<GuardrailState, string> = {
  pending: '… guardrail · pending',
  allow: '✓ guardrail · allow',
  deny: '✗ guardrail · deny',
};

export function ToolCallBubble({ tool, input, guardrailState }: ToolCallBubbleProps) {
  const inputJson = JSON.stringify(input);
  const truncated = inputJson.length > 220 ? inputJson.slice(0, 220) + '…' : inputJson;

  return (
    <div className="cp-row cp-row-tool">
      <div className="cp-tool">
        <div className="cp-tool-head">
          <span className="cp-tool-tag">tool</span>
          <span className="cp-tool-name">{tool}</span>
          <span className={`cp-guardrail ${guardrailState}`}>{BADGE_LABEL[guardrailState]}</span>
        </div>
        <pre className="cp-tool-input">{truncated}</pre>
      </div>
    </div>
  );
}
