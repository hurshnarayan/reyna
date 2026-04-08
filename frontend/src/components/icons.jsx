/*
 * Reyna Icon System — mirrors Monkeytype's Fa component
 * Uses Font Awesome 6 Free (solid/regular/brands)
 * All emoji→icon mappings centralized here.
 */

// ─── Fa Component (mirrors monkeytype/src/ts/components/common/Fa.tsx) ───

export function Fa({ icon, variant = 'solid', fw, spin, size, style, className = '' }) {
  const variantClass = variant === 'brand' ? 'fab' : variant === 'regular' ? 'far' : 'fas'
  const classes = [
    variantClass,
    icon,
    fw && 'fa-fw',
    spin && 'fa-spin',
    className,
  ].filter(Boolean).join(' ')

  return (
    <i
      className={classes}
      style={{
        fontSize: size ? `${size}em` : undefined,
        ...style,
      }}
    />
  )
}

// ─── Semantic icon map (emoji → FA icon) ───
// Every icon used across Reyna, mapped to the FA equivalent Monkeytype would use.

export const icons = {
  // Navigation / Layout
  dashboard:    'fa-chart-line',
  files:        'fa-folder',
  search:       'fa-search',
  bot:          'fa-terminal',
  alerts:       'fa-bell',
  settings:     'fa-cog',
  logout:       'fa-sign-out-alt',
  user:         'fa-user',

  // Dashboard stat cards
  totalFiles:   'fa-file-alt',
  groups:       'fa-users',
  storage:      'fa-database',
  cloud:        'fa-cloud',

  // Drive / Folder
  drive:        'fa-cloud',
  driveConnect: 'fa-link',
  folder:       'fa-folder',
  folderOpen:   'fa-folder-open',
  folderTree:   'fa-sitemap',
  changeFolder: 'fa-folder',

  // File types
  file:         'fa-file',
  filePdf:      'fa-file-pdf',
  fileImage:    'fa-file-image',
  fileWord:     'fa-file-word',
  fileExcel:    'fa-file-excel',
  filePpt:      'fa-file-powerpoint',
  fileZip:      'fa-file-archive',
  fileText:     'fa-file-alt',
  fileVideo:    'fa-video',
  fileAudio:    'fa-music',

  // Actions
  add:          'fa-plus',
  remove:       'fa-times',
  delete:       'fa-trash',
  edit:         'fa-pen',
  rename:       'fa-pen',
  download:     'fa-download',
  upload:       'fa-upload',
  preview:      'fa-eye',
  refresh:      'fa-sync-alt',
  commit:       'fa-arrow-right',
  save:         'fa-save',
  check:        'fa-check',
  close:        'fa-times',

  // Status / State
  staging:      'fa-clock',
  warning:      'fa-exclamation-triangle',
  error:        'fa-times',
  success:      'fa-check',
  notice:       'fa-info',
  loading:      'fa-circle-notch',
  trophy:       'fa-trophy',

  // Sidebar sections
  inbox:        'fa-inbox',
  announce:     'fa-bullhorn',
  notifHistory: 'fa-bell',

  // Misc
  star:         'fa-star',
  crown:        'fa-crown',
  lock:         'fa-lock',
  brain:        'fa-brain',
  comment:      'fa-comment-alt',
  timer:        'fa-clock',
  tag:          'fa-tag',
  pin:          'fa-thumbtack',
  link:         'fa-external-link-alt',
  tip:          'fa-lightbulb',
  send:         'fa-paper-plane',
  expand:       'fa-chevron-down',
  collapse:     'fa-chevron-up',
  chevronRight: 'fa-chevron-right',
  arrowRight:   'fa-arrow-right',

  // Tracking modes
  trackAll:     'fa-folder',
  trackReact:   'fa-thumbtack',
}

// ─── File icon resolver (replaces emoji-based fileIcon functions) ───

export function fileIconClass(mimeType, fileName) {
  const m = (mimeType || '').toLowerCase()
  const ext = (fileName || '').split('.').pop()?.toLowerCase()
  if (m.includes('pdf') || ext === 'pdf') return icons.filePdf
  if (m.includes('image') || ['png','jpg','jpeg','gif','webp'].includes(ext)) return icons.fileImage
  if (m.includes('word') || ['doc','docx'].includes(ext)) return icons.fileWord
  if (m.includes('spreadsheet') || m.includes('excel') || ['xls','xlsx','csv'].includes(ext)) return icons.fileExcel
  if (m.includes('presentation') || m.includes('powerpoint') || ['ppt','pptx'].includes(ext)) return icons.filePpt
  if (m.includes('zip') || m.includes('rar') || m.includes('tar') || ['zip','rar','7z','tar','gz'].includes(ext)) return icons.fileZip
  if (m.includes('text') || ['txt','md'].includes(ext)) return icons.fileText
  if (m.includes('video') || ['mp4','avi','mkv'].includes(ext)) return icons.fileVideo
  if (m.includes('audio') || ['mp3','wav','ogg'].includes(ext)) return icons.fileAudio
  return icons.file
}

// ─── Inline icon helper — renders an FA icon inside a styled container ───

export function IconBox({ icon, color, bg, size = 36, iconSize, rounded = 8, style }) {
  return (
    <div style={{
      width: size, height: size, borderRadius: rounded,
      background: bg || 'var(--card-bg)',
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      flexShrink: 0,
      ...style,
    }}>
      <Fa icon={icon} style={{ color: color || 'var(--main-color)', fontSize: iconSize || size * 0.42 }} />
    </div>
  )
}
