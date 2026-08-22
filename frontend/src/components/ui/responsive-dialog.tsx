import * as React from 'react'
import { Dialog } from '@/components/ui/dialog'
import { MobileSheet } from '@/components/ui/mobile-sheet'

interface ResponsiveDialogProps {
  open: boolean
  onClose: () => void
  title?: string
  children: React.ReactNode
}

/**
 * Every native-league transaction dialog (Assign player, Trade, Rollover
 * confirm, Edit contract) — centered modal at `md`+, full-screen `MobileSheet`
 * below it, same `open`/`onClose`/`children` either way so call sites don't
 * branch. Both shells render (each is a no-op when `open` is false); the
 * inactive one is `display:none`'d via the wrapping div rather than picked
 * with a media-query hook, matching how every other screen in the app
 * switches between its table and card-list variant (`hidden md:block` /
 * `md:hidden`) — `display:none` removes a shell from the accessibility tree
 * too, so there's no double-dialog risk for screen readers.
 */
export function ResponsiveDialog({ open, onClose, title, children }: ResponsiveDialogProps) {
  return (
    <>
      <div className="hidden md:contents">
        <Dialog open={open} onClose={onClose} title={title}>{children}</Dialog>
      </div>
      <div className="contents md:hidden">
        <MobileSheet open={open} onClose={onClose} title={title}>{children}</MobileSheet>
      </div>
    </>
  )
}
