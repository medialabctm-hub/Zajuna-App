import { Component, type ReactNode } from 'react'
import { PageError } from './AsyncState'

/**
 * Keeps a page failure (for example a page chunk that could not load) inside
 * the workspace, so the sidebar and header stay usable. React.lazy remembers a
 * failed import, so recovering needs a reload.
 */
export class PageErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false }

  static getDerivedStateFromError() {
    return { failed: true }
  }

  render() {
    if (this.state.failed) {
      return (
        <PageError
          message="Esta sección no se pudo abrir. Recarga la aplicación para intentarlo de nuevo."
          action={<button className="button" onClick={() => window.location.reload()}>Recargar</button>}
        />
      )
    }
    return this.props.children
  }
}
