import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { AboutModal } from '@/components/AboutModal';

// These tests derive expected behavior from the spec carried in the original
// design bundle (CapVariantC4 + "Animated Shimmer" header button) plus the
// agent tool surface advertised in About → Available tools. They verify the
// modal's a11y wiring, the verbatim copy (name/tagline/description), the four
// capability titles, the five-layer stack ordering, the agent tool surface
// (13 read + 5 write rows), and the "More information" link cards. The
// dismissal contract is also covered: Escape, backdrop, and X close; clicks
// inside the modal must not propagate.

const SPEC_NAME = 'EKS Control Plane';
const SPEC_TAGLINE = 'Agentic infrastructure operations, with guardrails';
const SPEC_DESCRIPTION =
  'A control plane for Amazon EKS that turns natural-language requests into safe, constrained Kubernetes operations. The agent decides intent; the backend enforces what may actually be executed.';

const SPEC_CAPABILITY_TITLES = ['Dashboard', 'Agent', 'Enforcer', 'Infrastructure'];

const SPEC_STACK_AREAS = ['Infrastructure', 'Backend', 'Agent runtime', 'Frontend', 'Delivery'];

// Mirrors the AGENT_TOOLS list rendered in About → Available tools.
// 13 read tools + 5 write tools = 18 total.
const SPEC_READ_TOOLS = [
  'cluster_info',
  'cluster_health',
  'list_namespaces',
  'list_nodes',
  'list_deployments',
  'get_deployment',
  'list_replicasets',
  'list_pods',
  'list_services',
  'list_ingresses',
  'list_hpas',
  'list_events',
  'tail_logs',
];

const SPEC_WRITE_TOOLS = [
  'scale',
  'rollout_restart',
  'pause_rollout',
  'resume_rollout',
  'rollback',
];

describe('AboutModal', () => {
  it('renders as a labeled modal dialog with the spec hero copy', () => {
    render(<AboutModal onClose={() => {}} />);

    const dialog = screen.getByRole('dialog');
    expect(dialog).toHaveAttribute('aria-modal', 'true');

    // The hero eyebrow signals the section's purpose.
    expect(within(dialog).getByText('About this project')).toBeInTheDocument();

    // Title is the labeled name — assert it's the heading, not just any node
    // that happens to contain "EKS Control Plane" (the header brand mark also
    // does, but the modal dialog scopes us away from it).
    const title = within(dialog).getByRole('heading', { level: 1, name: SPEC_NAME });
    expect(title).toBeInTheDocument();
    // aria-labelledby on the dialog must point at this heading.
    expect(dialog).toHaveAttribute('aria-labelledby', title.id);

    expect(within(dialog).getByText(SPEC_TAGLINE)).toBeInTheDocument();
    expect(within(dialog).getByText(SPEC_DESCRIPTION)).toBeInTheDocument();
  });

  it('renders all four capability titles', () => {
    render(<AboutModal onClose={() => {}} />);
    const dialog = screen.getByRole('dialog');
    // Scope to the capability cards — "Infrastructure" also appears as a
    // tech-stack area name, so a free-text lookup is ambiguous.
    const cardTitles = Array.from(dialog.querySelectorAll('.c4-card-title')).map(
      (node) => node.textContent,
    );
    expect(cardTitles).toEqual(SPEC_CAPABILITY_TITLES);
  });

  it('renders all five stack layers in spec order with 01..05 indices', () => {
    render(<AboutModal onClose={() => {}} />);
    const dialog = screen.getByRole('dialog');

    const layerNodes = dialog.querySelectorAll('.c4-stack-layer');
    expect(layerNodes.length).toBe(SPEC_STACK_AREAS.length);

    SPEC_STACK_AREAS.forEach((area, index) => {
      const layer = layerNodes[index] as HTMLElement;
      const expectedIdx = String(index + 1).padStart(2, '0');
      expect(within(layer).getByText(expectedIdx)).toBeInTheDocument();
      expect(within(layer).getByText(area)).toBeInTheDocument();
    });
  });

  it('renders eighteen tool rows, each with a kind badge and its tool name', () => {
    render(<AboutModal onClose={() => {}} />);
    const dialog = screen.getByRole('dialog');

    const rows = dialog.querySelectorAll('.c4-tool-row');
    expect(rows.length).toBe(SPEC_READ_TOOLS.length + SPEC_WRITE_TOOLS.length);
    expect(rows.length).toBe(18);

    for (const toolName of [...SPEC_READ_TOOLS, ...SPEC_WRITE_TOOLS]) {
      // Each tool name appears exactly once across rows.
      const matches = within(dialog).getAllByText(toolName);
      expect(matches.length).toBe(1);
      // The tool-name span lives inside a c4-tool-row.
      const row = matches[0].closest('.c4-tool-row');
      expect(row).not.toBeNull();
      // ...and the row contains a kind badge (read or write).
      const badge = (row as HTMLElement).querySelector('.c4-tool-kind');
      expect(badge).not.toBeNull();
      expect(['read', 'write']).toContain(badge!.textContent);
    }
  });

  it('renders exactly 13 read badges and 5 write badges in the rendered rows', () => {
    render(<AboutModal onClose={() => {}} />);
    const dialog = screen.getByRole('dialog');

    const badges = Array.from(dialog.querySelectorAll('.c4-tool-row .c4-tool-kind'));
    const readBadges = badges.filter((badge) => badge.textContent === 'read');
    const writeBadges = badges.filter((badge) => badge.textContent === 'write');

    expect(readBadges.length).toBe(13);
    expect(writeBadges.length).toBe(5);
  });

  it('renders the More information section with four external links', () => {
    render(<AboutModal onClose={() => {}} />);
    const dialog = screen.getByRole('dialog');

    const heading = within(dialog).getByRole('heading', { level: 2, name: 'More information' });
    const section = heading.closest('section') as HTMLElement;
    expect(section).not.toBeNull();

    const expected = [
      {
        title: 'GitHub repo',
        href: 'https://github.com/Junwei-Carl-Yao/agentic-eks-control-plane',
      },
      {
        title: 'Architecture',
        href: 'https://github.com/Junwei-Carl-Yao/agentic-eks-control-plane/blob/master/docs/architecture.md',
      },
      {
        title: 'Safety',
        href: 'https://github.com/Junwei-Carl-Yao/agentic-eks-control-plane/blob/master/docs/guardrails.md',
      },
      {
        title: 'LinkedIn',
        href: 'https://www.linkedin.com/in/junweiyao/',
      },
    ];

    const links = Array.from(section.querySelectorAll('a.c4-link'));
    expect(links.length).toBe(expected.length);

    expected.forEach((entry, index) => {
      const anchor = links[index] as HTMLAnchorElement;
      expect(anchor.getAttribute('href')).toBe(entry.href);
      expect(anchor.getAttribute('target')).toBe('_blank');
      expect(anchor.getAttribute('rel')).toBe('noopener noreferrer');
      expect(within(anchor).getByText(entry.title)).toBeInTheDocument();
    });
  });

  it('invokes onClose when the X close button is clicked', async () => {
    const onClose = vi.fn();
    render(<AboutModal onClose={onClose} />);

    const closeButton = screen.getByRole('button', { name: /close/i });
    await userEvent.click(closeButton);

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('invokes onClose when the backdrop is clicked', () => {
    const onClose = vi.fn();
    const { container } = render(<AboutModal onClose={onClose} />);

    const overlay = container.querySelector('.ab-overlay');
    expect(overlay).not.toBeNull();

    // The overlay's onClick is the close handler. Firing a click on the
    // overlay itself simulates a backdrop click (no target inside the modal).
    fireEvent.click(overlay as Element);

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('does NOT invoke onClose when a click happens inside the modal body', async () => {
    const onClose = vi.fn();
    render(<AboutModal onClose={onClose} />);

    const dialog = screen.getByRole('dialog');
    // Click on a clearly-interior element (the hero title heading).
    const interior = within(dialog).getByRole('heading', { level: 1, name: SPEC_NAME });
    await userEvent.click(interior);

    expect(onClose).not.toHaveBeenCalled();
  });

  it('invokes onClose when Escape is pressed', async () => {
    const onClose = vi.fn();
    render(<AboutModal onClose={onClose} />);

    await userEvent.keyboard('{Escape}');

    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
