import Hls from 'hls.js'
import type { HlsInstance, HlsPort } from './streamSource'

// hlsPort is the real hls.js behind the Stream Source's HlsPort seam.
export const hlsPort: HlsPort = {
  isSupported: () => Hls.isSupported(),
  create: () => new Hls() as unknown as HlsInstance,
  events: { error: Hls.Events.ERROR, manifestParsed: Hls.Events.MANIFEST_PARSED },
}
