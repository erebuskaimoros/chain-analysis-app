import type { ReactNode, SVGProps } from "react";

type IconProps = SVGProps<SVGSVGElement> & { size?: number };

function Icon({ size = 16, children, ...props }: IconProps & { children: ReactNode }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.5}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
      {...props}
    >
      {children}
    </svg>
  );
}

export const HomeIcon = (props: IconProps) => (
  <Icon {...props}>
    <path d="M2.5 7.2 8 2.5l5.5 4.7" />
    <path d="M4 6.2v7.3h3V10h2v3.5h3V6.2" />
  </Icon>
);

export const ActorsIcon = (props: IconProps) => (
  <Icon {...props}>
    <circle cx="6" cy="5.5" r="2.3" />
    <path d="M1.8 13.5c.5-2.3 2.2-3.6 4.2-3.6s3.7 1.3 4.2 3.6" />
    <path d="M10.5 3.4a2.2 2.2 0 0 1 0 4.2" />
    <path d="M12 10.1c1.2.4 2 1.5 2.2 3.4" />
  </Icon>
);

export const GraphIcon = (props: IconProps) => (
  <Icon {...props}>
    <circle cx="3.5" cy="4" r="1.8" />
    <circle cx="12.5" cy="3.5" r="1.8" />
    <circle cx="8" cy="12.5" r="1.8" />
    <path d="M5.2 4.6 10.8 3.9M4.5 5.6l2.6 5.2M11.7 5.1 8.9 10.9" />
  </Icon>
);

export const ExplorerIcon = (props: IconProps) => (
  <Icon {...props}>
    <circle cx="8" cy="8" r="5.8" />
    <path d="m10.4 5.6-1.5 3.3-3.3 1.5 1.5-3.3z" />
  </Icon>
);

export const TraceIcon = (props: IconProps) => (
  <Icon {...props}>
    <circle cx="3" cy="3.5" r="1.5" />
    <circle cx="13" cy="12.5" r="1.5" />
    <path d="M4.5 3.5h5a2.5 2.5 0 0 1 0 5h-3a2.5 2.5 0 0 0 0 5h5" />
  </Icon>
);

export const CasesIcon = (props: IconProps) => (
  <Icon {...props}>
    <path d="M1.8 4.2c0-.7.5-1.2 1.2-1.2h3l1.5 1.6H13c.7 0 1.2.5 1.2 1.2v6.9c0 .7-.5 1.3-1.2 1.3H3c-.7 0-1.2-.6-1.2-1.3z" />
  </Icon>
);

export const TagIcon = (props: IconProps) => (
  <Icon {...props}>
    <path d="M2 2.5h5.3l6.4 6.4-4.8 4.8L2.5 7.3z" />
    <circle cx="5.2" cy="5.6" r="1" />
  </Icon>
);

export const SearchIcon = (props: IconProps) => (
  <Icon {...props}>
    <circle cx="7" cy="7" r="4.5" />
    <path d="m10.4 10.4 3.4 3.4" />
  </Icon>
);

export const PlusIcon = (props: IconProps) => (
  <Icon {...props}>
    <path d="M8 3v10M3 8h10" />
  </Icon>
);

export const CloseIcon = (props: IconProps) => (
  <Icon {...props}>
    <path d="m4 4 8 8M12 4l-8 8" />
  </Icon>
);

export const ChevronDownIcon = (props: IconProps) => (
  <Icon {...props}>
    <path d="m4 6 4 4 4-4" />
  </Icon>
);

export const ChevronUpIcon = (props: IconProps) => (
  <Icon {...props}>
    <path d="m4 10 4-4 4 4" />
  </Icon>
);

export const ChevronRightIcon = (props: IconProps) => (
  <Icon {...props}>
    <path d="m6 4 4 4-4 4" />
  </Icon>
);

export const PanelLeftIcon = (props: IconProps) => (
  <Icon {...props}>
    <rect x="2" y="2.5" width="12" height="11" rx="1.5" />
    <path d="M6 2.5v11" />
  </Icon>
);

export const PanelRightIcon = (props: IconProps) => (
  <Icon {...props}>
    <rect x="2" y="2.5" width="12" height="11" rx="1.5" />
    <path d="M10 2.5v11" />
  </Icon>
);

export const MoreIcon = (props: IconProps) => (
  <Icon {...props}>
    <circle cx="3.5" cy="8" r="0.6" fill="currentColor" />
    <circle cx="8" cy="8" r="0.6" fill="currentColor" />
    <circle cx="12.5" cy="8" r="0.6" fill="currentColor" />
  </Icon>
);

export const CopyIcon = (props: IconProps) => (
  <Icon {...props}>
    <rect x="5.5" y="5.5" width="8" height="8" rx="1.3" />
    <path d="M10.5 3.5v-.3c0-.7-.5-1.2-1.2-1.2H3.7c-.7 0-1.2.5-1.2 1.2v5.6c0 .7.5 1.2 1.2 1.2h.3" />
  </Icon>
);

export const CheckIcon = (props: IconProps) => (
  <Icon {...props}>
    <path d="m3 8.5 3 3 7-7" />
  </Icon>
);

export const ExternalIcon = (props: IconProps) => (
  <Icon {...props}>
    <path d="M9.5 2.5h4v4M13.5 2.5 7.5 8.5" />
    <path d="M12 9.5v3c0 .6-.4 1-1 1H3.5c-.6 0-1-.4-1-1V5c0-.6.4-1 1-1h3" />
  </Icon>
);

export const TrashIcon = (props: IconProps) => (
  <Icon {...props}>
    <path d="M2.5 4.5h11M6 4.5V3h4v1.5M4 4.5l.6 8.3c0 .4.4.7.8.7h5.2c.4 0 .8-.3.8-.7l.6-8.3" />
  </Icon>
);

export const PlayIcon = (props: IconProps) => (
  <Icon {...props}>
    <path d="M4.5 3v10l8-5z" />
  </Icon>
);

export const RefreshIcon = (props: IconProps) => (
  <Icon {...props}>
    <path d="M13 3.5v3h-3" />
    <path d="M12.6 6.3A5 5 0 1 0 13 9.5" />
  </Icon>
);

export const DownloadIcon = (props: IconProps) => (
  <Icon {...props}>
    <path d="M8 2.5v8M4.8 7.5 8 10.7l3.2-3.2M2.5 13.5h11" />
  </Icon>
);

export const FileIcon = (props: IconProps) => (
  <Icon {...props}>
    <path d="M4 1.8h5l3 3v9.4H4z" />
    <path d="M9 1.8v3h3" />
  </Icon>
);

export const SunIcon = (props: IconProps) => (
  <Icon {...props}>
    <circle cx="8" cy="8" r="2.8" />
    <path d="M8 1.5v1.3M8 13.2v1.3M1.5 8h1.3M13.2 8h1.3M3.4 3.4l.9.9M11.7 11.7l.9.9M3.4 12.6l.9-.9M11.7 4.3l.9-.9" />
  </Icon>
);

export const MoonIcon = (props: IconProps) => (
  <Icon {...props}>
    <path d="M13.2 9.6A5.6 5.6 0 0 1 6.4 2.8a5.6 5.6 0 1 0 6.8 6.8z" />
  </Icon>
);

export const MonitorIcon = (props: IconProps) => (
  <Icon {...props}>
    <rect x="1.8" y="2.5" width="12.4" height="8.5" rx="1.2" />
    <path d="M6 13.5h4M8 11v2.5" />
  </Icon>
);

export const KeyboardIcon = (props: IconProps) => (
  <Icon {...props}>
    <rect x="1.5" y="4" width="13" height="8" rx="1.3" />
    <path d="M4 6.5h.01M6.5 6.5h.01M9 6.5h.01M11.5 6.5h.01M5 9.5h6" />
  </Icon>
);

export const PinIcon = (props: IconProps) => (
  <Icon {...props}>
    <path d="M9.8 2.2 13.8 6.2l-1.6.6-2.6 2.6-.3 2.8-1.1 1.1-5.3-5.3 1.1-1.1 2.8-.3 2.6-2.6z" />
    <path d="m5.3 10.7-3 3" />
  </Icon>
);

export const AlertIcon = (props: IconProps) => (
  <Icon {...props}>
    <path d="M8 2 14.5 13.5h-13z" />
    <path d="M8 6.5v3M8 11.6h.01" />
  </Icon>
);

export const StopIcon = (props: IconProps) => (
  <Icon {...props}>
    <rect x="4" y="4" width="8" height="8" rx="1.2" />
  </Icon>
);

export const ArrowRightIcon = (props: IconProps) => (
  <Icon {...props}>
    <path d="M2.5 8h11M9.5 4l4 4-4 4" />
  </Icon>
);

export const ArrowLeftIcon = (props: IconProps) => (
  <Icon {...props}>
    <path d="M13.5 8h-11M6.5 4l-4 4 4 4" />
  </Icon>
);

export const CollapseIcon = (props: IconProps) => (
  <Icon {...props}>
    <rect x="2" y="2.5" width="12" height="11" rx="1.5" />
    <path d="M6 2.5v11M10.5 6.5 9 8l1.5 1.5" />
  </Icon>
);

export const ExpandIcon = (props: IconProps) => (
  <Icon {...props}>
    <rect x="2" y="2.5" width="12" height="11" rx="1.5" />
    <path d="M6 2.5v11M9 6.5 10.5 8 9 9.5" />
  </Icon>
);

export const MenuIcon = (props: IconProps) => (
  <Icon {...props}>
    <path d="M2.5 4h11M2.5 8h11M2.5 12h11" />
  </Icon>
);

export const EditIcon = (props: IconProps) => (
  <Icon {...props}>
    <path d="M10.5 2.5 13.5 5.5 5.5 13.5H2.5v-3z" />
  </Icon>
);

export const EyeIcon = (props: IconProps) => (
  <Icon {...props}>
    <path d="M1.5 8S4 3.5 8 3.5 14.5 8 14.5 8 12 12.5 8 12.5 1.5 8 1.5 8z" />
    <circle cx="8" cy="8" r="2" />
  </Icon>
);

export function BrandMark({ size = 20 }: { size?: number }) {
  // Two flows meeting at a node: the app follows value across chains.
  return (
    <svg width={size} height={size} viewBox="0 0 20 20" aria-hidden="true" focusable="false">
      <path d="M2 5.5c4.5 0 5.5 4.5 8 4.5" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
      <path d="M2 14.5c4.5 0 5.5-4.5 8-4.5h8" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
      <circle cx="10" cy="10" r="2.6" fill="currentColor" />
    </svg>
  );
}
