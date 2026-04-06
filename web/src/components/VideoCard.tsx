import { Link } from 'react-router-dom'
import type { VideoWithTags } from '../types'
import { formatDuration, formatFileSize } from '../utils/format'

interface VideoCardProps {
  video: VideoWithTags
}

export default function VideoCard({ video }: VideoCardProps) {
  return (
    <Link
      to={`/videos/${video.id}`}
      className="group bg-gray-900 rounded-lg overflow-hidden hover:ring-2 hover:ring-indigo-500 transition-all"
    >
      <div className="aspect-video bg-gray-800 relative">
        {video.thumbnail_url ? (
          <img
            src={video.thumbnail_url}
            alt={video.title}
            className="w-full h-full object-cover"
            loading="lazy"
          />
        ) : (
          <div className="w-full h-full flex items-center justify-center text-gray-600">
            <svg className="w-12 h-12" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M15.75 10.5l4.72-4.72a.75.75 0 011.28.53v11.38a.75.75 0 01-1.28.53l-4.72-4.72M4.5 18.75h9a2.25 2.25 0 002.25-2.25v-9a2.25 2.25 0 00-2.25-2.25h-9A2.25 2.25 0 002.25 7.5v9a2.25 2.25 0 002.25 2.25z" />
            </svg>
          </div>
        )}
        <span className="absolute bottom-1 right-1 bg-black/80 text-white text-xs px-1.5 py-0.5 rounded">
          {formatDuration(video.duration_seconds)}
        </span>
      </div>
      <div className="p-3">
        <h3 className="text-sm text-white font-medium line-clamp-2 group-hover:text-indigo-400 transition-colors">
          {video.title}
        </h3>
        <div className="mt-1 flex items-center gap-2 text-xs text-gray-500">
          <span>{video.resolution}</span>
          <span>·</span>
          <span>{formatFileSize(video.file_size_bytes)}</span>
        </div>
        {video.tags.length > 0 && (
          <div className="mt-2 flex flex-wrap gap-1">
            {video.tags.slice(0, 3).map((tag) => (
              <span
                key={tag.id}
                className="text-xs bg-gray-800 text-gray-400 px-1.5 py-0.5 rounded"
              >
                {tag.name}
              </span>
            ))}
            {video.tags.length > 3 && (
              <span className="text-xs text-gray-600">+{video.tags.length - 3}</span>
            )}
          </div>
        )}
      </div>
    </Link>
  )
}
