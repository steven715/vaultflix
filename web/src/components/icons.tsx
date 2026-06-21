import type { SVGProps } from 'react'

// Shared inline SVG icons (Heroicons-style stroke + custom brand marks).
// All inherit currentColor so callers control color via text-*.

type IconProps = SVGProps<SVGSVGElement>

function Stroke({ children, ...props }: IconProps) {
  return (
    <svg
      fill="none"
      viewBox="0 0 24 24"
      stroke="currentColor"
      strokeWidth={1.8}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      {...props}
    >
      {children}
    </svg>
  )
}

// Brand: hexagon shield with inner hexagon.
export function LogoMark(props: IconProps) {
  return (
    <svg fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.6} aria-hidden="true" {...props}>
      <path strokeLinejoin="round" d="M12 2l9 5v10l-9 5-9-5V7z" />
      <path strokeLinejoin="round" d="M12 7l4.5 2.5v5L12 17l-4.5-2.5v-5z" />
    </svg>
  )
}

export const SearchIcon = (p: IconProps) => (
  <Stroke {...p}>
    <path d="M21 21l-5.2-5.2m2.2-5.3a7.5 7.5 0 11-15 0 7.5 7.5 0 0115 0z" />
  </Stroke>
)

export const HomeIcon = (p: IconProps) => (
  <Stroke {...p}>
    <path d="M2.25 12l8.954-8.955a1.126 1.126 0 011.591 0L21.75 12M4.5 9.75v10.125c0 .621.504 1.125 1.125 1.125H9.75v-4.875c0-.621.504-1.125 1.125-1.125h2.25c.621 0 1.125.504 1.125 1.125V21h4.125c.621 0 1.125-.504 1.125-1.125V9.75" />
  </Stroke>
)

export const HeartIcon = (p: IconProps) => (
  <Stroke {...p}>
    <path d="M21 8.25c0-2.485-2.099-4.5-4.688-4.5-1.935 0-3.597 1.126-4.312 2.733-.715-1.607-2.377-2.733-4.313-2.733C5.1 3.75 3 5.765 3 8.25c0 7.22 9 12 9 12s9-4.78 9-12z" />
  </Stroke>
)

export const HeartFilled = (p: IconProps) => (
  <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true" {...p}>
    <path d="M11.645 20.91l-.007-.003-.022-.012a15.247 15.247 0 01-.383-.218 25.18 25.18 0 01-4.244-3.17C4.688 15.36 2.25 12.174 2.25 8.25 2.25 5.322 4.714 3 7.688 3A5.5 5.5 0 0112 5.052 5.5 5.5 0 0116.313 3c2.973 0 5.437 2.322 5.437 5.25 0 3.925-2.438 7.111-4.739 9.256a25.175 25.175 0 01-4.244 3.17 15.247 15.247 0 01-.383.219l-.022.012-.007.004-.003.001a.752.752 0 01-.704 0l-.003-.001z" />
  </svg>
)

export const ClockIcon = (p: IconProps) => (
  <Stroke {...p}>
    <path d="M12 6v6h4.5m4.5 0a9 9 0 11-18 0 9 9 0 0118 0z" />
  </Stroke>
)

export const PlayIcon = (p: IconProps) => (
  <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true" {...p}>
    <path d="M5.25 5.653c0-.856.917-1.398 1.667-.986l11.54 6.348a1.125 1.125 0 010 1.971l-11.54 6.347a1.125 1.125 0 01-1.667-.985V5.653z" />
  </svg>
)

export const PauseIcon = (p: IconProps) => (
  <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true" {...p}>
    <path d="M6.75 5.25a.75.75 0 01.75-.75H9a.75.75 0 01.75.75v13.5a.75.75 0 01-.75.75H7.5a.75.75 0 01-.75-.75V5.25zm7.5 0A.75.75 0 0115 4.5h1.5a.75.75 0 01.75.75v13.5a.75.75 0 01-.75.75H15a.75.75 0 01-.75-.75V5.25z" />
  </svg>
)

export const ChevronLeft = (p: IconProps) => (
  <Stroke {...p}>
    <path d="M15.75 19.5L8.25 12l7.5-7.5" />
  </Stroke>
)

export const ChevronRight = (p: IconProps) => (
  <Stroke {...p}>
    <path d="M8.25 4.5l7.5 7.5-7.5 7.5" />
  </Stroke>
)

export const ChevronDown = (p: IconProps) => (
  <Stroke {...p}>
    <path d="M19.5 8.25l-7.5 7.5-7.5-7.5" />
  </Stroke>
)

export const CloseIcon = (p: IconProps) => (
  <Stroke {...p}>
    <path d="M6 18L18 6M6 6l12 12" />
  </Stroke>
)

export const UserIcon = (p: IconProps) => (
  <Stroke {...p}>
    <path d="M15.75 6a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0zM4.501 20.118a7.5 7.5 0 0114.998 0A17.933 17.933 0 0112 21.75c-2.676 0-5.216-.584-7.499-1.632z" />
  </Stroke>
)

export const LockIcon = (p: IconProps) => (
  <Stroke {...p}>
    <path d="M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" />
  </Stroke>
)

export const ShareIcon = (p: IconProps) => (
  <Stroke {...p}>
    <path d="M7.217 10.907a2.25 2.25 0 100 2.186m0-2.186c.18.324.283.696.283 1.093s-.103.77-.283 1.093m0-2.186l9.566-5.314m-9.566 7.5l9.566 5.314m0 0a2.25 2.25 0 103.935 2.186 2.25 2.25 0 00-3.935-2.186zm0-12.814a2.25 2.25 0 103.933-2.185 2.25 2.25 0 00-3.933 2.185z" />
  </Stroke>
)

export const CheckIcon = (p: IconProps) => (
  <Stroke {...p}>
    <path d="M4.5 12.75l6 6 9-13.5" />
  </Stroke>
)

export const VolumeIcon = (p: IconProps) => (
  <Stroke {...p}>
    <path d="M19.114 5.636a9 9 0 010 12.728M16.463 8.288a5.25 5.25 0 010 7.424M6.75 8.25l4.72-4.72a.75.75 0 011.28.531v15.878a.75.75 0 01-1.28.53l-4.72-4.72H4.51c-.88 0-1.704-.507-1.938-1.354A9.01 9.01 0 012.25 12c0-.83.112-1.633.322-2.395C2.806 8.756 3.63 8.25 4.51 8.25H6.75z" />
  </Stroke>
)

export const FullscreenIcon = (p: IconProps) => (
  <Stroke {...p}>
    <path d="M3.75 3.75v4.5m0-4.5h4.5m-4.5 0L9 9M3.75 20.25v-4.5m0 4.5h4.5m-4.5 0L9 15M20.25 3.75h-4.5m4.5 0v4.5m0-4.5L15 9m5.25 11.25h-4.5m4.5 0v-4.5m0 4.5L15 15" />
  </Stroke>
)
