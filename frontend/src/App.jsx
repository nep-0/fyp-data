import { useEffect, useMemo, useRef, useState } from 'react'
import {
  AlertTriangle,
  BookOpen,
  ChevronLeft,
  ChevronRight,
  ExternalLink,
  Filter,
  GraduationCap,
  Loader2,
  Mail,
  MapPin,
  MinusCircle,
  Search,
  Sparkles,
  Star,
  User,
  X,
} from 'lucide-react'
import './App.css'

const PAGE_SIZE = 12
const FAVORITES_KEY = 'fyp-theme-favorites'

const FILTER_DEFS = [
  { key: 'subject_area', dictType: 'theme_subject_area', label: 'Subject area' },
  { key: 'programme', dictType: 'theme_programme', label: 'Programme' },
  { key: 'project_type', dictType: 'theme_project_type', label: 'Project type' },
  { key: 'theme_type', dictType: 'theme_type', label: 'Theme type' },
]

function App() {
  const [health, setHealth] = useState(null)
  const [filters, setFilters] = useState({})
  const [dicts, setDicts] = useState({})
  const [query, setQuery] = useState('')
  const [submittedQuery, setSubmittedQuery] = useState('')
  const [negativeQuery, setNegativeQuery] = useState('')
  const [submittedNegativeQuery, setSubmittedNegativeQuery] = useState('')
  const [negativeOpen, setNegativeOpen] = useState(false)
  const [semantic, setSemantic] = useState(false)
  const [includeMissing, setIncludeMissing] = useState(false)
  const [offset, setOffset] = useState(0)
  const [results, setResults] = useState({ rows: [], total: 0 })
  const [selected, setSelected] = useState(null)
  const selectedRef = useRef(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const [favoriteIds, setFavoriteIds] = useState(() => readFavorites())
  const [showFavorites, setShowFavorites] = useState(false)
  const [showDataNotice, setShowDataNotice] = useState(true)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const favoriteKey = showFavorites ? favoriteIds.join(',') : ''

  useEffect(() => {
    selectedRef.current = selected
  }, [selected])

  useEffect(() => {
    let cancelled = false

    async function loadFilters() {
      try {
        const [healthData, ...dictResponses] = await Promise.all([
          api('/health'),
          ...FILTER_DEFS.map((def) =>
            api(`/dictionaries?type=${encodeURIComponent(def.dictType)}&limit=500`),
          ),
        ])
        if (cancelled) return
        setHealth(healthData)
        setDicts(
          FILTER_DEFS.reduce((next, def, index) => {
            next[def.key] = dictResponses[index].rows || []
            return next
          }, {}),
        )
        setSemantic(Boolean(healthData.semantic_available))
      } catch (err) {
        if (!cancelled) setError(err.message)
      }
    }

    loadFilters()
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    let cancelled = false

    async function loadThemes() {
      setLoading(true)
      setError('')
      try {
        const data = showFavorites
          ? await fetchFavoriteThemes(favoriteKey ? favoriteKey.split(',') : [])
          : await fetchThemes({
              query: submittedQuery,
              filters,
              offset,
              semantic,
              negativeQuery: submittedNegativeQuery,
              includeMissing,
            })
        if (cancelled) return
        setResults(data)
        const selectedID = selectedRef.current?.id
        const selectedVisible = data.rows.some((row) => row.id === selectedID)
        if ((!selectedID || !selectedVisible) && data.rows.length > 0) {
          selectTheme(data.rows[0])
        } else if (data.rows.length === 0) {
          setSelected(null)
          setDetailOpen(false)
        }
      } catch (err) {
        if (!cancelled) setError(err.message)
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    loadThemes()
    return () => {
      cancelled = true
    }
  }, [submittedQuery, submittedNegativeQuery, filters, offset, semantic, includeMissing, showFavorites, favoriteKey])

  const page = Math.floor(offset / PAGE_SIZE) + 1
  const totalPages = Math.max(1, Math.ceil((results.total || results.rows.length || 0) / PAGE_SIZE))
  const favoriteCount = favoriteIds.length
  const semanticActive = semantic && Boolean(submittedQuery)
  const canPage = !showFavorites && !semanticActive
  const canGoPrevious = canPage && offset > 0
  const canGoNext = canPage && offset + PAGE_SIZE < results.total
  const activeFilters = useMemo(
    () => Object.entries(filters).filter(([, value]) => value),
    [filters],
  )

  function submitSearch(event) {
    event.preventDefault()
    setOffset(0)
    setSubmittedQuery(query.trim())
    setSubmittedNegativeQuery(semantic ? negativeQuery.trim() : '')
  }

  function updateFilter(key, value) {
    setOffset(0)
    setShowFavorites(false)
    setFilters((current) => ({ ...current, [key]: value }))
  }

  function clearSearch() {
    setQuery('')
    setSubmittedQuery('')
    setSubmittedNegativeQuery('')
    setShowFavorites(false)
    setOffset(0)
  }

  function toggleFavorite(id) {
    setFavoriteIds((current) => {
      const idString = String(id)
      const next = current.includes(idString)
        ? current.filter((item) => item !== idString)
        : [idString, ...current]
      writeFavorites(next)
      return next
    })
  }

  function selectTheme(theme, options = {}) {
    setSelected(theme)
    if (options.open) setDetailOpen(true)
  }

  function goToPreviousPage() {
    if (canGoPrevious) setOffset(Math.max(0, offset - PAGE_SIZE))
  }

  function goToNextPage() {
    if (canGoNext) setOffset(offset + PAGE_SIZE)
  }

  return (
    <main className="app-shell">
      {showDataNotice && (
        <div className="startup-notice" role="presentation" onClick={() => setShowDataNotice(false)}>
          <div
            className="startup-dialog"
            role="dialog"
            aria-modal="true"
            aria-labelledby="startup-notice-title"
            onClick={(event) => event.stopPropagation()}
          >
            <AlertTriangle size={24} aria-hidden="true" />
            <div>
              <h2 id="startup-notice-title">Data may not be up to date</h2>
              <p>Theme availability can change. Check missing markers and confirm details at fyp-gc.uestc.edu.cn before making a decision.</p>
            </div>
            <button type="button" className="primary-button" onClick={() => setShowDataNotice(false)}>
              Continue
            </button>
          </div>
        </div>
      )}

      <section className="search-band">
        <div className="search-header">
          <div className="search-copy">
            <p className="eyebrow">FYP Theme Finder</p>
          </div>
          <a
            className="repo-button"
            href="https://github.com/nep-0/fyp-data"
            target="_blank"
            rel="noreferrer"
          >
            <ExternalLink size={17} aria-hidden="true" />
            GitHub
          </a>
        </div>

        <form className="search-box" onSubmit={submitSearch}>
          <Search aria-hidden="true" />
          <input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Search by topic, teacher, department, or skill"
          />
          {query && (
            <button type="button" className="icon-button" onClick={clearSearch} aria-label="Clear search">
              <X size={18} aria-hidden="true" />
            </button>
          )}
          <button type="submit" className="primary-button">
            <Search size={18} aria-hidden="true" />
            Search
          </button>
        </form>

        <div className="toolbar">
          <div className="filter-row">
            <Filter size={18} aria-hidden="true" />
            {FILTER_DEFS.map((def) => (
              <label key={def.key} className="select-field">
                <span>{def.label}</span>
                <select
                  value={filters[def.key] || ''}
                  onChange={(event) => updateFilter(def.key, event.target.value)}
                >
                  <option value="">Any</option>
                  {(dicts[def.key] || []).map((row) => (
                    <option key={`${def.key}-${row.dictValue}`} value={row.dictValue}>
                      {displayDictionary(row)}
                    </option>
                  ))}
                </select>
              </label>
            ))}
          </div>

          <div className="toolbar-actions">
            <div className={`negative-search ${negativeOpen ? 'open' : ''}`}>
              <button
                type="button"
                className="negative-toggle"
                disabled={!semantic || !health?.semantic_available}
                onClick={() => setNegativeOpen((open) => !open)}
              >
                <MinusCircle size={16} aria-hidden="true" />
                Negative prompt
              </button>
              {negativeOpen && (
                <div className="negative-field">
                  <input
                    value={negativeQuery}
                    disabled={!semantic || !health?.semantic_available}
                    onChange={(event) => setNegativeQuery(event.target.value)}
                    placeholder="Optional negative prompt"
                  />
                  {negativeQuery && (
                    <button
                      type="button"
                      className="icon-button"
                      onClick={() => {
                        setNegativeQuery('')
                        setSubmittedNegativeQuery('')
                      }}
                      aria-label="Clear negative prompt"
                    >
                      <X size={16} aria-hidden="true" />
                    </button>
                  )}
                </div>
              )}
            </div>

            <label className={`semantic-toggle ${!health?.semantic_available ? 'disabled' : ''}`}>
              <input
                type="checkbox"
                checked={semantic}
                disabled={!health?.semantic_available}
                onChange={(event) => {
                  setOffset(0)
                  setSemantic(event.target.checked)
                }}
              />
              <Sparkles size={17} aria-hidden="true" />
              Semantic search
            </label>

            <label className="include-missing-toggle">
              <input
                type="checkbox"
                checked={includeMissing}
                onChange={(event) => {
                  setOffset(0)
                  setIncludeMissing(event.target.checked)
                }}
              />
              Include missing
            </label>
          </div>
        </div>

        {activeFilters.length > 0 && (
          <div className="chips">
            {activeFilters.map(([key, value]) => (
              <button key={key} type="button" onClick={() => updateFilter(key, '')}>
                {filterName(key)}: {filterValue(dicts[key], value)}
                <X size={14} aria-hidden="true" />
              </button>
            ))}
          </div>
        )}
      </section>

      <section className="content-grid">
        <div className="results-pane">
          <div className="pane-heading">
            <div>
              <h2>
                {showFavorites
                  ? 'Favorite themes'
                  : submittedQuery
                    ? `Results for "${submittedQuery}"`
                    : 'Available themes'}
              </h2>
              <p>{loading ? 'Searching themes...' : resultSummary(results, semanticActive, showFavorites)}</p>
            </div>
            <div className="result-actions">
              <button
                type="button"
                className={`favorites-toggle ${showFavorites ? 'active' : ''}`}
                onClick={() => {
                  setOffset(0)
                  setShowFavorites((current) => !current)
                }}
              >
                <Star size={16} aria-hidden="true" />
                Favorites
                <span>{favoriteCount}</span>
              </button>
              <div className="pager">
                <button
                  type="button"
                  className="icon-button"
                  disabled={!canGoPrevious}
                  onClick={goToPreviousPage}
                  aria-label="Previous page"
                >
                  <ChevronLeft size={18} aria-hidden="true" />
                </button>
                <span>{showFavorites ? 'Saved' : semanticActive ? 'Top matches' : `${page} / ${totalPages}`}</span>
                <button
                  type="button"
                  className="icon-button"
                  disabled={!canGoNext}
                  onClick={goToNextPage}
                  aria-label="Next page"
                >
                  <ChevronRight size={18} aria-hidden="true" />
                </button>
              </div>
            </div>
          </div>

          {error && <div className="notice">{error}</div>}
          {loading ? (
            <div className="loading-state">
              <Loader2 className="spin" aria-hidden="true" />
              <span>Loading themes</span>
            </div>
          ) : (
            <div className="theme-list">
              {results.rows.map((item) => (
                <ThemeCard
                  key={`${item.id}-${item.similarity || ''}`}
                  item={item}
                  active={selected?.id === item.id}
                  favorite={favoriteIds.includes(String(item.id))}
                  missing={isMissing(item)}
                  onClick={() => selectTheme(item, { open: true })}
                  onToggleFavorite={() => toggleFavorite(item.id)}
                />
              ))}
              {results.rows.length === 0 && (
                <div className="empty-state">
                  <BookOpen aria-hidden="true" />
                  <h3>{showFavorites ? 'No favorite themes yet' : 'No matching themes'}</h3>
                  <p>
                    {showFavorites
                      ? 'Use the star on any theme to save it here.'
                      : 'Try a broader keyword or remove one of the filters.'}
                  </p>
                </div>
              )}
              {canPage && results.rows.length > 0 && (
                <div className="bottom-pager">
                  <button type="button" className="page-button" disabled={!canGoPrevious} onClick={goToPreviousPage}>
                    <ChevronLeft size={18} aria-hidden="true" />
                    Previous
                  </button>
                  <span>
                    Page {page} of {totalPages}
                  </span>
                  <button type="button" className="page-button" disabled={!canGoNext} onClick={goToNextPage}>
                    Next
                    <ChevronRight size={18} aria-hidden="true" />
                  </button>
                </div>
              )}
            </div>
          )}
        </div>

        <div
          className={`detail-overlay ${selected && detailOpen ? 'open' : ''}`}
          onClick={() => setDetailOpen(false)}
        >
          <ThemeDetail
            theme={selected}
            favorite={selected ? favoriteIds.includes(String(selected.id)) : false}
            onClose={() => setDetailOpen(false)}
            onToggleFavorite={selected ? () => toggleFavorite(selected.id) : undefined}
          />
        </div>
      </section>
    </main>
  )
}

function ThemeCard({ item, active, favorite, missing, onClick, onToggleFavorite }) {
  const theme = item.theme || item
  return (
    <article className={`theme-card ${active ? 'active' : ''} ${missing ? 'missing' : ''}`}>
      <button type="button" className="theme-card-main" onClick={onClick}>
        <div className="card-topline">
          <span>{labelFor(theme, 'themeSubjectArea') || 'Uncategorized'}</span>
          <div>
            {missing && <em>Missing</em>}
            {item.similarity != null && <strong>{Math.round(item.similarity * 100)}% match</strong>}
          </div>
        </div>
        <h3>{theme.themeTitle || 'Untitled theme'}</h3>
        <p className="card-description">{plainText(theme.themeProjectDescription)}</p>
        <div className="card-meta">
          <span>
            <User size={15} aria-hidden="true" />
            {theme.teacherPinyin || theme.teacherName || 'Unknown teacher'}
          </span>
          {teacherRank(theme) && <span className="rank-badge">{teacherRank(theme)}</span>}
          <span>
            <GraduationCap size={15} aria-hidden="true" />
            {labelFor(theme, 'themeProjectType') || 'Project'}
          </span>
        </div>
      </button>
      <button
        type="button"
        className={`favorite-button ${favorite ? 'active' : ''}`}
        onClick={onToggleFavorite}
        aria-label={favorite ? 'Remove from favorites' : 'Add to favorites'}
      >
        <Star size={18} aria-hidden="true" />
      </button>
    </article>
  )
}

function ThemeDetail({ theme, favorite, onClose, onToggleFavorite }) {
  if (!theme) {
    return (
      <aside className="detail-pane center-detail">
        <BookOpen aria-hidden="true" />
        <p>Select a theme to see the details.</p>
      </aside>
    )
  }

  const badges = [
    labelFor(theme, 'themeType'),
    labelFor(theme, 'themeProjectType'),
    departmentName(theme),
    isMissing(theme) ? 'Missing' : '',
  ].filter(Boolean)

  return (
    <aside className={`detail-pane ${isMissing(theme) ? 'missing' : ''}`} onClick={(event) => event.stopPropagation()}>
      <button type="button" className="detail-close" onClick={onClose} aria-label="Close theme details">
        <X size={19} aria-hidden="true" />
      </button>
      <div className="detail-header">
        <p>{labelFor(theme, 'themeSubjectAreaSub') || labelFor(theme, 'themeSubjectArea')}</p>
        <div className="detail-title-row">
          <h2>{theme.themeTitle || 'Untitled theme'}</h2>
          <button
            type="button"
            className={`favorite-button detail-favorite ${favorite ? 'active' : ''}`}
            onClick={onToggleFavorite}
            aria-label={favorite ? 'Remove from favorites' : 'Add to favorites'}
          >
            <Star size={19} aria-hidden="true" />
          </button>
        </div>
        <div className="badge-row">
          {badges.map((badge) => (
            <span key={badge}>{badge}</span>
          ))}
        </div>
      </div>

      <div className="teacher-panel">
        <div className="avatar">{initials(theme.teacherPinyin || theme.teacherName)}</div>
        <div>
          <h3>{theme.teacherPinyin || theme.teacherName || 'Unknown teacher'}</h3>
          <div className="teacher-subline">
            <p>{theme.teacherName && theme.teacherPinyin ? theme.teacherName : theme.deptName_en || theme.deptName}</p>
            {teacherRank(theme) && <span className="rank-badge">{teacherRank(theme)}</span>}
          </div>
        </div>
      </div>

      <dl className="info-grid">
        <Info icon={<GraduationCap size={17} />} label="Programme" value={programmes(theme)} />
        <Info icon={<MapPin size={17} />} label="Office" value={theme.themeOfficeLocation} />
        <Info icon={<Mail size={17} />} label="Email" value={theme.teacherEmail || theme.email} />
        <Info icon={<User size={17} />} label="Places" value={theme.themeCount ? String(theme.themeCount) : ''} />
      </dl>

      <section className="detail-section">
        <h3>Project Description</h3>
        <RichText html={theme.themeProjectDescription} />
      </section>

      <section className="detail-section">
        <h3>Prerequisite Skills</h3>
        <RichText html={theme.themePrerequisiteSkills} />
      </section>
    </aside>
  )
}

function Info({ icon, label, value }) {
  if (!value) return null
  return (
    <div>
      <dt>
        {icon}
        {label}
      </dt>
      <dd>{value}</dd>
    </div>
  )
}

function RichText({ html }) {
  const clean = sanitizeHTML(html)
  if (!clean) return <p className="muted">No description provided.</p>
  return <div className="rich-text" dangerouslySetInnerHTML={{ __html: clean }} />
}

async function fetchThemes({ query, filters, offset, semantic, negativeQuery, includeMissing }) {
  if (semantic && query) {
    const params = new URLSearchParams({ q: query, limit: String(PAGE_SIZE) })
    if (negativeQuery) params.set('negative', negativeQuery)
    const data = await api(`/semantic-search?${params.toString()}`)
    const rows = (data.rows || [])
      .map((row) => ({ ...row.theme, similarity: row.similarity }))
      .filter((theme) => matchesFilters(theme, filters))
      .filter((theme) => includeMissing || !isMissing(theme))
    return {
      rows,
      total: rows.length,
    }
  }

  const params = new URLSearchParams({ limit: String(PAGE_SIZE), offset: String(offset) })
  Object.entries(filters).forEach(([key, value]) => {
    if (value) params.set(key, value)
  })
  if (!includeMissing) params.set('missing', 'false')
  const path = query ? '/search' : '/themes'
  if (query) params.set('q', query)
  return normalizeThemeList(await api(`${path}?${params.toString()}`))
}

async function fetchFavoriteThemes(favoriteIds) {
  const rows = await Promise.all(favoriteIds.map((id) => api(`/themes/${encodeURIComponent(id)}`)))
  return {
    rows,
    total: rows.length,
  }
}

function matchesFilters(theme, filters) {
  return Object.entries(filters).every(([key, value]) => {
    if (!value) return true
    if (key === 'subject_area') return theme.themeSubjectArea === value
    if (key === 'programme') return theme.themeProgramme.split(',').map((part) => part.trim()).includes(value)
    if (key === 'project_type') return theme.themeProjectType === value
    if (key === 'theme_type') return theme.themeType === value
    return true
  })
}

function isMissing(theme) {
  return theme?.missing === true || theme?.missing === 1 || theme?.missing === 'true' || theme?.missing === '1'
}

function normalizeThemeList(data) {
  const rows = Array.isArray(data.rows) ? data.rows : []
  return {
    ...data,
    rows,
    total: Number.isFinite(data.total) ? data.total : rows.length,
  }
}

async function api(path) {
  const response = await fetch(path)
  const data = await response.json().catch(() => ({}))
  if (!response.ok) {
    throw new Error(data.error || `Request failed with ${response.status}`)
  }
  return data
}

function displayDictionary(row) {
  return row.dictLabelEn || row.dictLabel || row.label || row.dictValue
}

function labelFor(theme, field) {
  const value = theme.labels?.[field]
  if (Array.isArray(value)) return value.map((item) => item.label_en || item.label).filter(Boolean).join(', ')
  if (value && typeof value === 'object') return value.label_en || value.label
  return ''
}

function programmes(theme) {
  return labelFor(theme, 'themeProgramme') || theme.themeProgramme
}

function teacherRank(theme) {
  return labelFor(theme, 'extendJb') || theme.extendJb
}

function departmentName(theme) {
  return theme.deptName_en || theme.deptName
}

function plainText(html) {
  return sanitizeHTML(html).replace(/<[^>]*>/g, ' ').replace(/\s+/g, ' ').trim()
}

function sanitizeHTML(html = '') {
  return String(html)
    .replace(/<script[\s\S]*?>[\s\S]*?<\/script>/gi, '')
    .replace(/\son\w+="[^"]*"/gi, '')
    .replace(/\son\w+='[^']*'/gi, '')
    .replace(/\s(href|src)=["']javascript:[^"']*["']/gi, '')
}

function initials(name = '') {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return '?'
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase()
  return `${parts[0][0]}${parts[parts.length - 1][0]}`.toUpperCase()
}

function filterName(key) {
  return FILTER_DEFS.find((def) => def.key === key)?.label || key
}

function filterValue(rows = [], value) {
  const row = rows.find((item) => item.dictValue === value)
  return row ? displayDictionary(row) : value
}

function resultSummary(results, semantic, showFavorites) {
  if (showFavorites) return `${results.rows.length} saved ${results.rows.length === 1 ? 'theme' : 'themes'}`
  if (semantic) return `${results.rows.length} strongest matches`
  const total = results.total || 0
  return `${total} ${total === 1 ? 'theme' : 'themes'} found`
}

function readFavorites() {
  try {
    const parsed = JSON.parse(window.localStorage.getItem(FAVORITES_KEY) || '[]')
    if (!Array.isArray(parsed)) return []
    return parsed.map((id) => String(id)).filter(Boolean)
  } catch {
    return []
  }
}

function writeFavorites(ids) {
  window.localStorage.setItem(FAVORITES_KEY, JSON.stringify(ids))
}

export default App
