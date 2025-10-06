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
    <div className="fixed inset-0 z-50 bg-slate-950/80 backdrop-blur-sm flex items-center justify-center p-6">
      <div className="relative w-full max-w-6xl bg-slate-900 border border-slate-800 rounded-2xl shadow-2xl p-8 space-y-6">
        <header className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
          <div>
            <h2 className="text-xl font-semibold text-slate-100">CSV Explorer</h2>
            <p className="text-sm text-slate-400">Browse uploaded CSV batches. Select one to load its results.</p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-2 rounded-lg border border-slate-700 text-slate-300 hover:bg-slate-800"
          >
            Close
          </button>
        </header>

        {items.length === 0 ? (
          <p className="text-sm text-slate-400">No CSV batches yet. Upload a CSV to get started.</p>
        ) : (
          <div className="grid gap-6 grid-cols-2 sm:grid-cols-3 lg:grid-cols-4">
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
                    'relative aspect-square rounded-2xl border border-slate-800 bg-slate-900/80 p-4 text-left transition-transform hover:-translate-y-1 hover:border-brand-500',
                    isSelected && 'ring-2 ring-brand-500 border-brand-500'
                  )}
                >
                  <div className="flex flex-col h-full justify-between">
                    <div className="space-y-2">
                      <div className="h-12 w-12 rounded-xl bg-gradient-to-br from-brand-500 to-brand-400/80 flex items-center justify-center text-white font-semibold text-lg">
                        {initialsFor(batch.name)}
                      </div>
                      <h3 className="text-sm font-semibold text-slate-100 line-clamp-2">{batch.name}</h3>
                      <p className="text-xs text-slate-400">Owner: {batch.owner}</p>
                    </div>
                    <div className="text-xs text-slate-400 space-y-1">
                      <p>Total: {batch.row_count.toLocaleString()}</p>
                      <p>Evaluated: {batch.processed_domains.toLocaleString()}</p>
                      <p>Remaining: {remaining.toLocaleString()}</p>
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
