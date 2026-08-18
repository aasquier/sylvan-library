/**
 * The pieces the two logged-out screens share: a card to sit in, a field, and
 * the countdown a 429 owes.
 *
 * Login and claim are the only screens in the app that render for somebody the
 * server will not answer any other question for, so they share a shape
 * deliberately: no nav, no health line, nothing that would need a fetch that is
 * about to come back 401.
 */

/** The panel both screens are, so they cannot drift apart visually. */
export function AuthCard({ title, blurb, children }: {
  title: string
  blurb?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <div className="card-surface rounded-xl p-6">
      <h1 className="text-lg font-semibold tracking-tight">{title}</h1>
      {blurb && (
        <p className="mt-1 text-sm leading-relaxed" style={{ color: 'var(--text-secondary)' }}>
          {blurb}
        </p>
      )}
      <div className="mt-5">{children}</div>
    </div>
  )
}

export function AuthField({
  label, value, onChange, type = 'text', autoComplete, autoFocus, hint, required = true,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  type?: 'text' | 'password' | 'email'
  autoComplete?: string
  autoFocus?: boolean
  hint?: string
  required?: boolean
}) {
  return (
    <label className="flex flex-col gap-1">
      <span className="text-[11px] font-medium uppercase tracking-wide"
            style={{ color: 'var(--text-muted)' }}>
        {label}
      </span>
      <input
        type={type}
        value={value}
        required={required}
        autoComplete={autoComplete}
        // Autofocus is usually a nuisance and here it is not: a screen whose
        // entire content is one form has nowhere else for the caret to start.
        autoFocus={autoFocus}
        onChange={(e) => onChange(e.target.value)}
        aria-label={label}
        className="h-9 rounded-md px-2 text-sm outline-none focus:ring-2"
        style={{ background: 'var(--surface-1)', color: 'var(--text-primary)',
                 border: '1px solid var(--hairline)' }}
      />
      {hint && (
        <span className="text-xs" style={{ color: 'var(--text-muted)' }}>{hint}</span>
      )}
    </label>
  )
}

/** The submit button every auth form uses, with its own busy state shown. */
export function AuthSubmit({ label, busyLabel, busy, disabled }: {
  label: string
  busyLabel: string
  busy: boolean
  disabled?: boolean
}) {
  return (
    <button type="submit" disabled={busy || disabled}
            className="btn btn-primary">
      {busy ? busyLabel : label}
    </button>
  )
}

/**
 * A statement of fact that is neither success nor failure.
 *
 * `ErrorNote` is red and the Accounts page's note is green; the reset endpoint's
 * answer must be neither, because it says nothing about whether anything
 * happened. Rendering it as a success would be the UI asserting what ADR 16
 * built the endpoint not to say.
 */
export function PlainNote({ children }: { children: React.ReactNode }) {
  return (
    <div className="rounded-lg px-4 py-3 text-sm leading-relaxed"
         style={{ background: 'var(--surface-1)',
                  border: '1px solid var(--hairline)',
                  color: 'var(--text-secondary)' }}>
      {children}
    </div>
  )
}
