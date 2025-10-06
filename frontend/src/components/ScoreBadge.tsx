import clsx from 'clsx';

interface ScoreBadgeProps {
  label: string | number;
  variant: 'trademark' | 'vice' | 'overall';
  score?: number;
}

export default function ScoreBadge({ label, variant, score }: ScoreBadgeProps) {
	return (
		<span
			className={clsx(
				'inline-flex items-center justify-center px-2 py-1 text-xs font-semibold rounded-full capitalize',
				getPalette(variant, label, score)
			)}
		>
			{label}
		</span>
	);
}

function getPalette(variant: ScoreBadgeProps['variant'], label: string | number, score?: number) {
	if (variant === 'overall') {
		switch (String(label).toUpperCase()) {
			case 'BLOCK':
				return 'bg-red-500/20 text-red-300 border border-red-500/30';
			case 'REVIEW':
				return 'bg-sky-500/20 text-sky-300 border border-sky-500/30';
			case 'ALLOW_WITH_CAUTION':
				return 'bg-amber-500/20 text-amber-300 border border-amber-500/30';
			default:
				return 'bg-emerald-500/20 text-emerald-300 border border-emerald-500/30';
		}
	}

  const palettes: Record<'trademark' | 'vice', string[]> = {
    trademark: [
      'bg-slate-700 text-slate-200',
      'bg-amber-500/20 text-amber-200 border border-amber-500/30',
      'bg-amber-500/30 text-amber-200 border border-amber-500/40',
      'bg-orange-500/30 text-orange-200 border border-orange-500/40',
      'bg-red-500/30 text-red-200 border border-red-500/40',
      'bg-red-500/40 text-red-100 border border-red-500/50'
    ],
    vice: [
      'bg-slate-700 text-slate-200',
      'bg-purple-500/20 text-purple-200 border border-purple-500/30',
      'bg-purple-500/30 text-purple-200 border border-purple-500/40',
      'bg-fuchsia-500/30 text-fuchsia-200 border border-fuchsia-500/40',
      'bg-rose-500/40 text-rose-100 border border-rose-500/40',
      'bg-rose-600/40 text-rose-50 border border-rose-600/50'
    ]
  };

  const palette = palettes[variant];
  const index = typeof score === 'number' && score >= 0 && score < palette.length ? score : 0;
	return palette[index as number] ?? palette[0];
}
