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
				'inline-flex items-center justify-center px-2.5 py-1 text-[11px] font-semibold rounded-full capitalize border',
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
				return 'bg-[#ffe9e4] text-[#8f2f1d] border-[#f2c7bd]';
			case 'REVIEW':
				return 'bg-[#e7f0ff] text-[#1e4fbf] border-[#c5d9ff]';
			case 'ALLOW_WITH_CAUTION':
				return 'bg-[#fff4da] text-[#8a5a00] border-[#f1d79b]';
			default:
				return 'bg-[#e6f7ee] text-[#1f6b3d] border-[#bfe7cf]';
		}
	}

  const palettes: Record<'trademark' | 'vice', string[]> = {
    trademark: [
      'bg-[#f3f1ec] text-[#6a655c] border-[#ded8cd]',
      'bg-[#fff4da] text-[#8a5a00] border-[#f1d79b]',
      'bg-[#ffe7c2] text-[#8a4e00] border-[#f1c78a]',
      'bg-[#ffd6b2] text-[#8a3c00] border-[#f3b285]',
      'bg-[#ffc1b2] text-[#8a2f2a] border-[#f0a59b]',
      'bg-[#ffb0a6] text-[#7a1e1a] border-[#ec9187]'
    ],
    vice: [
      'bg-[#f3f1ec] text-[#6a655c] border-[#ded8cd]',
      'bg-[#e9f2ff] text-[#1e4fbf] border-[#c7dcff]',
      'bg-[#d7e9ff] text-[#1d46a7] border-[#b4d0ff]',
      'bg-[#c6ddff] text-[#153b90] border-[#9fc2ff]',
      'bg-[#b6d2ff] text-[#0f3177] border-[#8bb3ff]',
      'bg-[#a3c4ff] text-[#0b2b6b] border-[#7fa8ff]'
    ]
  };

  const palette = palettes[variant];
  const index = typeof score === 'number' && score >= 0 && score < palette.length ? score : 0;
	return palette[index as number] ?? palette[0];
}
