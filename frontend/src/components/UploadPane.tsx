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
    <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-5 shadow-sm">
      <header className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-sm font-semibold text-slate-100">Dataset</h2>
          <p className="text-xs text-slate-400">
            Upload a CSV or run the evaluation for the latest batch already on the server.
          </p>
        </div>
        <button
          type="button"
          onClick={handleSubmit}
          disabled={busy || processing}
          className={clsx(
            'rounded-full px-4 py-1.5 text-sm font-medium transition-colors',
            busy || processing
              ? 'bg-slate-700 text-slate-400 cursor-not-allowed'
              : 'bg-slate-100 text-slate-900 hover:bg-white'
          )}
        >
          {processing || busy ? 'Working…' : csvFile ? 'Upload & evaluate' : 'Evaluate batch'}
        </button>
      </header>

      <div className="mt-5 space-y-3 text-sm">
        <label className="flex flex-col gap-1 text-slate-300">
          <span className="text-xs uppercase tracking-wide text-slate-500">Domains CSV</span>
          <input
            type="file"
            accept=".csv"
            onChange={(event) => setCsvFile(event.target.files?.[0] ?? null)}
            className="block w-full rounded-lg border border-slate-700 bg-slate-950/40 px-3 py-2 text-sm text-slate-200 file:mr-3 file:rounded-md file:border-0 file:bg-slate-800 file:px-3 file:py-2 file:text-slate-200 hover:file:bg-slate-700"
          />
          {csvFile && <span className="text-xs text-slate-500">{csvFile.name}</span>}
        </label>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <label className="flex flex-col gap-1 text-slate-300">
            <span className="text-xs uppercase tracking-wide text-slate-500">Batch name</span>
            <input
              type="text"
              value={batchName}
              onChange={(event) => setBatchName(event.target.value)}
              placeholder="e.g. October upload"
              className="rounded-lg border border-slate-700 bg-slate-950/40 px-3 py-2 text-sm text-slate-200 placeholder:text-slate-500 focus:border-slate-200 focus:outline-none"
            />
          </label>
          <label className="flex flex-col gap-1 text-slate-300">
            <span className="text-xs uppercase tracking-wide text-slate-500">Owner</span>
            <input
              type="text"
              value={ownerName}
              onChange={(event) => setOwnerName(event.target.value)}
              placeholder="Team or person"
              className="rounded-lg border border-slate-700 bg-slate-950/40 px-3 py-2 text-sm text-slate-200 placeholder:text-slate-500 focus:border-slate-200 focus:outline-none"
            />
          </label>
        </div>
      </div>

      {lastResponse && (
        <div className="mt-6 rounded-2xl border border-slate-800 bg-slate-950/50 p-6 text-sm text-slate-100 shadow-sm">
          <div className="flex flex-col gap-1">
            <p className="text-sm font-semibold text-slate-200">{lastResponse.batch_name}</p>
            <p className="text-xs uppercase tracking-wide text-slate-500">Upload summary</p>
          </div>
          <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            <Stat label="Rows" value={lastResponse.row_count} />
            <Stat label="Unique domains" value={lastResponse.unique_domains} />
            <Stat label="Already evaluated" value={lastResponse.processed_domains} />
            <Stat label="Duplicates" value={lastResponse.duplicate_rows} />
            <Stat label="Existing matches" value={lastResponse.existing_domains} />
            <Stat label="Marks in DB" value={lastResponse.marks_count} />
          </div>
        </div>
      )}

      {error && <p className="mt-4 text-xs text-red-400">{error}</p>}
    </section>
  );
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className="flex flex-col rounded-xl border border-slate-800/60 bg-slate-900/60 px-4 py-3">
      <span className="text-xs uppercase tracking-wide text-slate-500">{label}</span>
      <span className="text-base font-semibold text-slate-100 leading-tight">{value.toLocaleString()}</span>
    </div>
  );
}
