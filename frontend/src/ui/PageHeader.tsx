import type { ReactNode } from "react";

interface PageHeaderProps {
  title: string;
  context?: ReactNode;
  actions?: ReactNode;
}

// PageHeader is the one line at the top of every page: its name, what it is
// currently showing, and the page's main actions.
export function PageHeader({ title, context, actions }: PageHeaderProps) {
  return (
    <header className="page-header">
      <div className="page-header-title">
        <h1>{title}</h1>
        {context ? <span className="page-header-context">{context}</span> : null}
      </div>
      {actions ? <div className="page-header-actions">{actions}</div> : null}
    </header>
  );
}
