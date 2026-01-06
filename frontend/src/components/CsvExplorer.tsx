import clsx from 'clsx';
import type { BatchDTO } from '../types';

interface CsvExplorerProps {
  open: boolean;
  batches: BatchDTO[];
  selectedId?: number | null;
  onSelect: (batch: BatchDTO) => void;
  onClose: () => void;
}

export default function CsvExplorer({ open, batches, selectedId, onSelect, onClose }: CsvExplorerProps) {
  if (!open) return null;

  const items = Array.isArray(batches) ? batches : [];

  return (
    <div className="fixed inset-0 z-50 bg-black/30 backdrop-blur-sm flex items-center justify-center p-6">
      <div className="relative w-full max-w-5xl rounded-3xl border border-[var(--line)] bg-[var(--surface)] shadow-[0_30px_80px_rgba(10,10,10,0.15)] p-8 space-y-6">
        <header className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
          <div>
            <h2 className="text-xl font-semibold text-[var(--text)]">Datasets</h2>
            <p className="text-sm text-[var(--muted)]">Select a batch to load its results.</p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="rounded-full border border-[var(--line)] px-4 py-2 text-[11px] font-semibold uppercase tracking-[0.2em] text-[var(--text)] hover:bg-[var(--surface-2)]"
          >
            Close
          </button>
        </header>

        {items.length === 0 ? (
          <p className="text-sm text-[var(--muted)]">No CSV batches yet. Upload a CSV to get started.</p>
        ) : (
          <div className="space-y-3">
            {items.map((batch) => {
              const isSelected = batch.id === selectedId;
              const remaining = Math.max(batch.unique_domains - batch.processed_domains, 0);
              return (
                <button
                  key={batch.id}
                  type="button"
                  onClick={() => {
                    onSelect(batch);
                    onClose();
                  }}
                  className={clsx(
                    'w-full rounded-2xl border px-5 py-4 text-left transition-all',
                    isSelected
                      ? 'border-[var(--text)] bg-[var(--surface-2)] shadow-[0_10px_25px_rgba(10,10,10,0.08)]'
                      : 'border-[var(--line)] bg-white hover:-translate-y-0.5 hover:shadow-[0_10px_25px_rgba(10,10,10,0.08)]'
                  )}
                >
                  <div className="flex flex-wrap items-center justify-between gap-4">
                    <div className="flex items-center gap-4">
                      <div className="h-10 w-10 rounded-2xl bg-[var(--text)] text-white flex items-center justify-center text-sm font-semibold">
                        {initialsFor(batch.name)}
                      </div>
                      <div>
                        <h3 className="text-sm font-semibold text-[var(--text)] break-words">{batch.name}</h3>
                        <p className="text-xs text-[var(--muted)]">Owner: {batch.owner}</p>
                      </div>
                    </div>
                    <div className="text-xs text-[var(--muted)] flex flex-wrap gap-4">
                      <span>Total {batch.row_count.toLocaleString()}</span>
                      <span>Evaluated {batch.processed_domains.toLocaleString()}</span>
                      <span>Remaining {remaining.toLocaleString()}</span>
                    </div>
                  </div>
                </button>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}

function initialsFor(value: string) {
  if (!value) return 'CSV';
  const parts = value
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2);
  if (parts.length === 0) return 'CSV';
  return parts
    .map((part) => part.charAt(0).toUpperCase())
    .join('');
}
