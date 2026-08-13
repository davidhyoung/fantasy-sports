import { useState } from 'react'

interface Props {
  value: string
  onCommit: (next: string) => void
  placeholder?: string
  className?: string
}

/**
 * Free-text note that writes on blur or Enter rather than per keystroke — a
 * request per character would be one write per letter typed. Escape reverts.
 */
export function NoteField({ value, onCommit, placeholder = 'Note…', className = '' }: Props) {
  const [draft, setDraft] = useState(value)
  // Adopt server/optimistic changes that didn't come from this field.
  const [seen, setSeen] = useState(value)
  if (seen !== value) {
    setSeen(value)
    setDraft(value)
  }

  return (
    <input
      value={draft}
      onChange={(e) => setDraft(e.target.value)}
      onBlur={() => {
        if (draft !== value) onCommit(draft)
      }}
      onKeyDown={(e) => {
        if (e.key === 'Enter') e.currentTarget.blur()
        if (e.key === 'Escape') setDraft(value)
      }}
      placeholder={placeholder}
      maxLength={500}
      className={`w-full bg-transparent text-xs text-muted-foreground placeholder:text-muted-foreground/40 focus:text-foreground focus-visible:outline-none ${className}`}
    />
  )
}
