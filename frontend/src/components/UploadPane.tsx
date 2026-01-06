import { useState } from 'react';
import clsx from 'clsx';
import type { UploadResponse } from '../types';

interface UploadPaneProps {
  onProcess: (formData: FormData) => Promise<UploadResponse>;
  onEvaluate: (options?: { resume?: boolean; force?: boolean; batchId?: number }) => Promise<void>;
  busy: boolean;
}

export default function UploadPane({ onProcess, onEvaluate, busy }: UploadPaneProps) {
  const [csvFile, setCsvFile] = useState<File | null>(null);
  const [batchName, setBatchName] = useState('');
  const [ownerName, setOwnerName] = useState('');
  const [lastResponse, setLastResponse] = useState<UploadResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [processing, setProcessing] = useState(false);

  const handleSubmit = async () => {
    if (busy || processing) return;
    try {
      setProcessing(true);
      setError(null);

      const hasUploads = Boolean(csvFile);
      if (!hasUploads) {
        await onEvaluate();
        return;
      }

      if (!batchName.trim()) {
        setError('Enter a batch name.');
        return;
      }
      if (!ownerName.trim()) {
        setError('Enter an owner name.');
        return;
      }

      const form = new FormData();
      if (csvFile) form.append('domains', csvFile);
      form.append('batch_name', batchName.trim());
      form.append('owner_name', ownerName.trim());
      const response = await onProcess(form);
      setLastResponse(response);
      await onEvaluate({ batchId: response.batch_id });
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Upload failed';
      setError(message);
    } finally {
      setProcessing(false);
    }
  };

  return (
    <section className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-5">
      <header className="flex items-start justify-between gap-4">
        <div>
          <h2 className="font-semibold text-[var(--text)]">Upload Dataset</h2>
          <p className="text-sm text-[var(--muted)]">
            Upload a CSV or run the current batch.
          </p>
        </div>
        <button
          type="button"
          onClick={handleSubmit}
          disabled={busy || processing}
          className={clsx(
            'rounded-lg px-4 py-2 text-sm font-medium transition-all',
            busy || processing
              ? 'bg-[var(--surface-2)] text-[var(--muted)] cursor-not-allowed'
              : 'bg-[var(--text)] text-white hover:opacity-90'
          )}
        >
          {processing || busy ? 'Working…' : csvFile ? 'Upload & Run' : 'Run Batch'}
        </button>
      </header>

      <div className="mt-4 space-y-3">
        <label className="flex flex-col gap-1.5 text-[var(--text)]">
          <span className="text-sm font-medium text-[var(--muted)]">Domains CSV</span>
          <input
            type="file"
            accept=".csv"
            onChange={(event) => setCsvFile(event.target.files?.[0] ?? null)}
            className="block w-full rounded-lg border border-[var(--line)] bg-[var(--surface-2)] px-3 py-2.5 text-sm text-[var(--text)] file:mr-3 file:rounded-md file:border-0 file:bg-[var(--text)] file:px-3 file:py-1.5 file:text-sm file:font-medium file:text-white hover:file:opacity-90"
          />
          {csvFile && <span className="text-sm text-[var(--muted)]">{csvFile.name}</span>}
        </label>

        <div className="grid grid-cols-2 gap-3">
          <label className="flex flex-col gap-1.5 text-[var(--text)]">
            <span className="text-sm font-medium text-[var(--muted)]">Batch Name</span>
            <input
              type="text"
              value={batchName}
              onChange={(event) => setBatchName(event.target.value)}
              placeholder="e.g. October upload"
              className="rounded-lg border border-[var(--line)] bg-[var(--surface-2)] px-3 py-2.5 text-sm text-[var(--text)] placeholder:text-[var(--muted)] focus:border-[var(--accent)] focus:outline-none"
            />
          </label>
          <label className="flex flex-col gap-1.5 text-[var(--text)]">
            <span className="text-sm font-medium text-[var(--muted)]">Owner</span>
            <input
              type="text"
              value={ownerName}
              onChange={(event) => setOwnerName(event.target.value)}
              placeholder="Team or person"
              className="rounded-lg border border-[var(--line)] bg-[var(--surface-2)] px-3 py-2.5 text-sm text-[var(--text)] placeholder:text-[var(--muted)] focus:border-[var(--accent)] focus:outline-none"
            />
          </label>
        </div>
      </div>

      {lastResponse && (
        <div className="mt-4 rounded-lg border border-[var(--line)] bg-[var(--surface-2)] p-4">
          <div className="flex items-center justify-between mb-3">
            <p className="font-medium text-[var(--text)]">{lastResponse.batch_name}</p>
            <span className="text-sm text-[var(--muted)]">Upload complete</span>
          </div>
          <div className="grid grid-cols-2 gap-2">
            <Stat label="Rows" value={lastResponse.row_count} />
            <Stat label="Unique" value={lastResponse.unique_domains} />
            <Stat label="Evaluated" value={lastResponse.processed_domains} />
            <Stat label="Duplicates" value={lastResponse.duplicate_rows} />
          </div>
        </div>
      )}

      {error && <p className="mt-3 text-sm text-[var(--danger)]">{error}</p>}
    </section>
  );
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className="flex items-center justify-between rounded-md bg-[var(--surface)] px-3 py-2 border border-[var(--line)]">
      <span className="text-sm text-[var(--muted)]">{label}</span>
      <span className="font-semibold text-[var(--text)]">{value.toLocaleString()}</span>
    </div>
  );
}
