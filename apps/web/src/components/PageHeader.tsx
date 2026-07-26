import React from 'react';

interface PageHeaderProps {
  /** Small uppercase eyebrow label — the section's category. */
  label: string;
  title: string;
  description?: string;
  /** Optional right-aligned stats or status. */
  meta?: React.ReactNode;
}

/**
 * The single page-header pattern used across every view. Deliberately
 * typographic rather than a decorated hero block: eyebrow, title, one line
 * of description, hairline rule.
 */
export const PageHeader: React.FC<PageHeaderProps> = ({ label, title, description, meta }) => (
  <header className="mb-6">
    <div className="flex flex-wrap items-end justify-between gap-x-6 gap-y-2 pb-4 border-b border-white/[0.08]">
      <div className="min-w-0">
        <div className="font-mono text-[10px] uppercase tracking-[0.14em] text-emerald-400/80 mb-1.5">{label}</div>
        <h1 className="text-[22px] leading-tight font-semibold text-slate-50 tracking-[-0.01em]">{title}</h1>
        {description && <p className="text-[13px] text-slate-500 mt-1.5 max-w-2xl">{description}</p>}
      </div>
      {meta && <div className="shrink-0 pb-0.5">{meta}</div>}
    </div>
  </header>
);
