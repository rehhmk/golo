import React from 'react';

interface SparklineProps {
  /** Series of values, oldest → newest. Rendered as-is (no smoothing). */
  values: number[];
  /** Fixed value range so sparklines stay comparable across cards. */
  min?: number;
  max?: number;
  className?: string;
  stroke?: string;
  height?: number;
}

/**
 * Minimal inline SVG sparkline. Deliberately not Recharts — this renders
 * dozens of times on the board and needs to be near-free, with no axes,
 * tooltips, or layout cost.
 */
export const Sparkline: React.FC<SparklineProps> = ({
  values,
  min = 0,
  max = 100,
  className,
  stroke = 'currentColor',
  height = 28,
}) => {
  if (values.length < 2) {
    return <div style={{ height }} className={className} />;
  }

  const width = 100; // viewBox units; scales to container via preserveAspectRatio
  const range = max - min || 1;
  const stepX = width / (values.length - 1);

  const points = values.map((v, i) => {
    const clamped = Math.max(min, Math.min(max, v));
    const x = i * stepX;
    const y = height - ((clamped - min) / range) * height;
    return [x, y] as const;
  });

  const linePath = points.map(([x, y], i) => `${i === 0 ? 'M' : 'L'}${x.toFixed(2)},${y.toFixed(2)}`).join(' ');
  const areaPath = `${linePath} L${width},${height} L0,${height} Z`;
  const gradientId = React.useId();

  return (
    <svg
      viewBox={`0 0 ${width} ${height}`}
      preserveAspectRatio="none"
      className={className}
      style={{ height, width: '100%', display: 'block', color: stroke }}
      aria-hidden="true"
    >
      <defs>
        <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="currentColor" stopOpacity="0.18" />
          <stop offset="100%" stopColor="currentColor" stopOpacity="0" />
        </linearGradient>
      </defs>
      <path d={areaPath} fill={`url(#${gradientId})`} />
      <path d={linePath} fill="none" stroke="currentColor" strokeWidth="1.25" vectorEffect="non-scaling-stroke" />
    </svg>
  );
};
