import clsx from 'clsx';

interface ScoreBadgeProps {
  label: string;
  variant?: 'decision';
}

export default function ScoreBadge({ label }: ScoreBadgeProps) {
  return (
    <span
      className={clsx(
        'inline-flex items-center justify-center px-3 py-1.5 text-xs font-semibold rounded-full',
        getPalette(label)
      )}
    >
      {formatLabel(label)}
    </span>
  );
}

function formatLabel(label: string): string {
  switch (String(label).toUpperCase()) {
    case 'YES_RISK':
      return 'Yes Risk';
    case 'NO_RISK':
      return 'No Risk';
    case 'POTENTIAL_RISK':
      return 'Potential Risk';
    // Legacy format support
    case 'BLOCK':
      return 'Yes Risk';
    case 'REVIEW':
    case 'ALLOW_WITH_CAUTION':
      return 'Potential Risk';
    case 'ALLOW':
      return 'No Risk';
    default:
      return label;
  }
}

function getPalette(label: string): string {
  switch (String(label).toUpperCase()) {
    case 'YES_RISK':
    case 'BLOCK':
      // Red - definite risk
      return 'bg-red-100 text-red-800 border border-red-200';
    case 'NO_RISK':
    case 'ALLOW':
      // Green - safe
      return 'bg-green-100 text-green-800 border border-green-200';
    case 'POTENTIAL_RISK':
    case 'REVIEW':
    case 'ALLOW_WITH_CAUTION':
      // Amber - needs review
      return 'bg-amber-100 text-amber-800 border border-amber-200';
    default:
      return 'bg-gray-100 text-gray-800 border border-gray-200';
  }
}
