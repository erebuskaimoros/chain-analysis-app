import { Component, type ErrorInfo, type ReactNode } from "react";

interface PageErrorBoundaryProps {
  pageLabel: string;
  children: ReactNode;
}

interface PageErrorBoundaryState {
  error: Error | null;
}

// PageErrorBoundary keeps one page's crash from blanking the whole app: the
// navigation stays usable and the page can be reloaded in place.
export class PageErrorBoundary extends Component<PageErrorBoundaryProps, PageErrorBoundaryState> {
  state: PageErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): PageErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error(`The ${this.props.pageLabel} page crashed`, error, info.componentStack);
  }

  render() {
    if (!this.state.error) {
      return this.props.children;
    }
    return (
      <>
        <header className="page-header">
          <div className="page-header-title">
            <h1>{this.props.pageLabel}</h1>
          </div>
        </header>
        <div className="page-body page-body-narrow">
          <div className="empty-state" role="alert">
            <strong>This page stopped working</strong>
            <span>
              {this.props.pageLabel} hit an error and could not finish drawing. Your saved actors, cases and labels are not
              affected.
            </span>
            <code className="mono-wrap">{this.state.error.message}</code>
            <div className="form-actions">
              <button type="button" className="btn btn-primary" onClick={() => this.setState({ error: null })}>
                Try again
              </button>
              <button type="button" className="btn" onClick={() => window.location.reload()}>
                Reload the app
              </button>
            </div>
          </div>
        </div>
      </>
    );
  }
}
