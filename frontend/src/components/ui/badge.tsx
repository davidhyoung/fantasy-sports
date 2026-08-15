import * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/utils'

/**
 * Badges are tinted, never solid-filled — the only solid fill in the system is
 * the primary CTA button. Pill radius, Inter 600 at 11px.
 */
const badgeVariants = cva(
  'inline-flex items-center rounded-pill border px-2.5 py-1 text-[11px] font-semibold focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2',
  {
    variants: {
      variant: {
        // Tonal variants — the design system's badge vocabulary.
        pink: 'bg-positive-light text-positive-foreground border-positive-border',
        blue: 'bg-negative-light text-negative-foreground border-transparent',
        neutral: 'bg-muted text-muted-foreground border-transparent',
        // Reserved for projected/future data.
        purple: 'bg-highlight-light text-highlight-foreground border-highlight-border',
        amber: 'bg-warning-light text-warning-foreground border-warning-border',
        outline: 'text-foreground',
        // Solid fills — kept for existing call sites; prefer the tonal variants.
        default: 'border-transparent bg-primary text-primary-foreground hover:bg-primary/80',
        secondary: 'border-transparent bg-secondary text-secondary-foreground hover:bg-secondary/80',
        destructive: 'border-transparent bg-destructive text-destructive-foreground hover:bg-destructive/80',
      },
    },
    defaultVariants: {
      variant: 'neutral',
    },
  }
)

export interface BadgeProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return (
    <div className={cn(badgeVariants({ variant }), className)} {...props} />
  )
}

export { Badge, badgeVariants }
