import { useState } from 'react';
import clsx from 'clsx';
import type { UploadResponse } from '../types';

interface UploadPaneProps {
  onProcess: (formData: FormData) => Promise<UploadResponse>;
  onEvaluate: (resume?: boolean) => Promise<void>;
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
        setError('Please provide a batch name.');
        return;
      }
      if (!ownerName.trim()) {
        setError('Please provide an owner name.');
        return;
      }

      const form = new FormData();
      if (csvFile) form.append('domains', csvFile);
      form.append('batch_name', batchName.trim());
      form.append('owner_name', ownerName.trim());
      const response = await onProcess(form);
      setLastResponse(response);
      await onEvaluate();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Upload failed';
      setError(message);
    } finally {
      setProcessing(false);
    }
  };

  return (
    <section className="bg-slate-900 border border-slate-800 rounded-xl p-6 shadow-lg">
      <header className="flex items-center justify-between gap-4 mb-6">
        <div>
          <h2 className="text-lg font-semibold">Input Files</h2>
          <p className="text-sm text-slate-400">Upload a domains CSV or rely on the cached catalog already on the server.</p>
        </div>
        <button
          type="button"
          onClick={handleSubmit}
          disabled={busy || processing}
          className={clsx(
            'px-4 py-2 rounded-lg font-medium transition-colors',
            busy || processing
              ? 'bg-slate-700 text-slate-400 cursor-not-allowed'
              : 'bg-brand-500 hover:bg-brand-600 text-white'
          )}
        >
          {processing || busy ? 'Processing…' : 'Process & Evaluate'}
        </button>
      </header>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-6 mb-6">
        <label className="flex flex-col gap-2 text-sm">
          <span className="text-slate-300 font-medium">Batch Name</span>
          <input
            type="text"
            value={batchName}
            onChange={(event) => setBatchName(event.target.value)}
            placeholder="e.g. Demo Upload"
            className="bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-brand-500"
          />
        </label>
        <label className="flex flex-col gap-2 text-sm">
          <span className="text-slate-300 font-medium">Owner Name</span>
          <input
            type="text"
            value={ownerName}
            onChange={(event) => setOwnerName(event.target.value)}
            placeholder="Who owns this CSV?"
            className="bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-brand-500"
          />
        </label>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-6">
        <FileInput label="Domains CSV" accept=".csv" onSelect={setCsvFile} selected={csvFile?.name} />
      </div>

      {lastResponse && (
        <dl className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 mt-6 text-sm">
          <Stat label="Batch" value={lastResponse.batch_name} helper={`Owner: ${lastResponse.owner}`} />
          <Stat label="Rows" value={lastResponse.row_count.toLocaleString()} helper={`Unique: ${lastResponse.unique_domains.toLocaleString()}`} />
          <Stat label="Duplicates" value={lastResponse.duplicate_rows.toLocaleString()} helper={`Existing: ${lastResponse.existing_domains.toLocaleString()}`} />
          <Stat label="Already Evaluated" value={lastResponse.processed_domains.toLocaleString()} helper="Will be reused" />
          <Stat label="Marks In DB" value={lastResponse.marks_count.toLocaleString()} />
        </dl>
      )}

      {error && <p className="mt-4 text-sm text-red-400">{error}</p>}
    </section>
  );
}

interface FileInputProps {
  label: string;
  accept: string;
  selected?: string;
  onSelect: (file: File | null) => void;
}

function FileInput({ label, accept, selected, onSelect }: FileInputProps) {
  return (
    <label className="flex flex-col gap-2 text-sm">
      <span className="text-slate-300 font-medium">{label}</span>
      <input
        type="file"
        accept={accept}
        onChange={(event) => onSelect(event.target.files?.[0] ?? null)}
        className="block w-full text-sm text-slate-200 file:mr-4 file:py-2 file:px-4 file:rounded-md file:border-0 file:text-sm file:font-semibold file:bg-slate-700 file:text-slate-200 hover:file:bg-slate-600"
      />
      {selected && <span className="text-xs text-slate-500">{selected}</span>}
    </label>
  );
}

function Stat({ label, value, helper }: { label: string; value: string | number; helper?: string }) {
  return (
    <div className="bg-slate-800 rounded-lg p-4">
      <dt className="text-slate-400">{label}</dt>
      <dd className="text-xl font-semibold">{value}</dd>
      {helper && <p className="text-xs text-slate-500 mt-1">{helper}</p>}
    </div>
  );
}
