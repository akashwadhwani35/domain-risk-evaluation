import { useState, useRef, useEffect } from 'react';
import clsx from 'clsx';

interface TooltipProps {
  content: string;
  children: React.ReactNode;
  position?: 'top' | 'bottom' | 'left' | 'right';
}

export default function Tooltip({ content, children, position = 'top' }: TooltipProps) {
  const [show, setShow] = useState(false);
  const tooltipRef = useRef<HTMLDivElement>(null);

  const positionClasses = {
    top: 'bottom-full left-1/2 -translate-x-1/2 mb-2',
    bottom: 'top-full left-1/2 -translate-x-1/2 mt-2',
    left: 'right-full top-1/2 -translate-y-1/2 mr-2',
    right: 'left-full top-1/2 -translate-y-1/2 ml-2'
  };

  const arrowClasses = {
    top: 'top-full left-1/2 -translate-x-1/2 border-t-gray-900 border-x-transparent border-b-transparent',
    bottom: 'bottom-full left-1/2 -translate-x-1/2 border-b-gray-900 border-x-transparent border-t-transparent',
    left: 'left-full top-1/2 -translate-y-1/2 border-l-gray-900 border-y-transparent border-r-transparent',
    right: 'right-full top-1/2 -translate-y-1/2 border-r-gray-900 border-y-transparent border-l-transparent'
  };

  return (
    <div
      className="relative inline-flex"
      onMouseEnter={() => setShow(true)}
      onMouseLeave={() => setShow(false)}
    >
      {children}
      {show && (
        <div
          ref={tooltipRef}
          className={clsx(
            'absolute z-50 px-3 py-2 text-xs text-white bg-gray-900 rounded-lg shadow-lg',
            'whitespace-normal max-w-xs',
            'animate-in fade-in duration-150',
            positionClasses[position]
          )}
        >
          {content}
          <div
            className={clsx(
              'absolute w-0 h-0 border-4',
              arrowClasses[position]
            )}
          />
        </div>
      )}
    </div>
  );
}

// Help icon with tooltip - the "?" button
interface HelpTipProps {
  content: string;
  position?: 'top' | 'bottom' | 'left' | 'right';
}

export function HelpTip({ content, position = 'top' }: HelpTipProps) {
  return (
    <Tooltip content={content} position={position}>
      <span className="inline-flex items-center justify-center w-4 h-4 text-[10px] font-medium text-[var(--muted)] bg-[var(--surface-2)] rounded-full cursor-help hover:bg-[var(--line)] hover:text-[var(--text)] transition-colors">
        ?
      </span>
    </Tooltip>
  );
}
