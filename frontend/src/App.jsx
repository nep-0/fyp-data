import { useEffect, useMemo, useRef, useState } from 'react'
import {
  BookOpen,
  ChevronLeft,
  ChevronRight,
  Filter,
  GraduationCap,
  Loader2,
  Mail,
  MapPin,
  MinusCircle,
  Search,
  Sparkles,
  User,
  X,
} from 'lucide-react'
import './App.css'

const PAGE_SIZE = 12

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
  const [offset, setOffset] = useState(0)
  const [results, setResults] = useState({ rows: [], total: 0 })
  const [selected, setSelected] = useState(null)
  const selectedRef = useRef(null)
  const [loading, setLoading] = useState(true)
  const [detailLoading, setDetailLoading] = useState(false)
  const [error, setError] = useState('')

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
        const data = await fetchThemes({
          query: submittedQuery,
          filters,
          offset,
          semantic,
          negativeQuery: submittedNegativeQuery,
        })
        if (cancelled) return
        setResults(data)
        const selectedID = selectedRef.current?.id
        const selectedVisible = data.rows.some((row) => row.id === selectedID)
        if ((!selectedID || !selectedVisible) && data.rows.length > 0) {
          await loadTheme(data.rows[0].id, { silent: true })
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
  }, [submittedQuery, submittedNegativeQuery, filters, offset, semantic])

  const page = Math.floor(offset / PAGE_SIZE) + 1
  const totalPages = Math.max(1, Math.ceil((results.total || results.rows.length || 0) / PAGE_SIZE))
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
    setFilters((current) => ({ ...current, [key]: value }))
  }

  function clearSearch() {
    setQuery('')
    setSubmittedQuery('')
    setSubmittedNegativeQuery('')
    setOffset(0)
  }

  async function loadTheme(id, options = {}) {
    if (!options.silent) setDetailLoading(true)
    setError('')
    try {
      const data = await api(`/themes/${id}`)
      setSelected(data)
    } catch (err) {
      setError(err.message)
    } finally {
      setDetailLoading(false)
    }
  }

  return (
    <main className="app-shell">
      <section className="search-band">
        <div className="search-copy">
          <p className="eyebrow">FYP Theme Finder</p>
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
              <h2>{submittedQuery ? `Results for "${submittedQuery}"` : 'Available themes'}</h2>
              <p>{loading ? 'Searching themes...' : resultSummary(results, semantic)}</p>
            </div>
            <div className="pager">
              <button
                type="button"
                className="icon-button"
                disabled={offset === 0 || semantic}
                onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
                aria-label="Previous page"
              >
                <ChevronLeft size={18} aria-hidden="true" />
              </button>
              <span>{semantic ? 'Top matches' : `${page} / ${totalPages}`}</span>
              <button
                type="button"
                className="icon-button"
                disabled={semantic || offset + PAGE_SIZE >= results.total}
                onClick={() => setOffset(offset + PAGE_SIZE)}
                aria-label="Next page"
              >
                <ChevronRight size={18} aria-hidden="true" />
              </button>
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
                  onClick={() => loadTheme(item.id)}
                />
              ))}
              {results.rows.length === 0 && (
                <div className="empty-state">
                  <BookOpen aria-hidden="true" />
                  <h3>No matching themes</h3>
                  <p>Try a broader keyword or remove one of the filters.</p>
                </div>
              )}
            </div>
          )}
        </div>

        <ThemeDetail theme={selected} loading={detailLoading} />
      </section>
    </main>
  )
}

function ThemeCard({ item, active, onClick }) {
  const theme = item.theme || item
  return (
    <button type="button" className={`theme-card ${active ? 'active' : ''}`} onClick={onClick}>
      <div className="card-topline">
        <span>{labelFor(theme, 'themeSubjectArea') || 'Uncategorized'}</span>
        {item.similarity != null && <strong>{Math.round(item.similarity * 100)}% match</strong>}
      </div>
      <h3>{theme.themeTitle || 'Untitled theme'}</h3>
      <p className="card-description">{plainText(theme.themeProjectDescription)}</p>
      <div className="card-meta">
        <span>
          <User size={15} aria-hidden="true" />
          {theme.teacherPinyin || theme.teacherName || 'Unknown teacher'}
        </span>
        <span>
          <GraduationCap size={15} aria-hidden="true" />
          {labelFor(theme, 'themeProjectType') || 'Project'}
        </span>
      </div>
    </button>
  )
}

function ThemeDetail({ theme, loading }) {
  if (loading) {
    return (
      <aside className="detail-pane center-detail">
        <Loader2 className="spin" aria-hidden="true" />
      </aside>
    )
  }

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
    labelFor(theme, 'themeState'),
  ].filter(Boolean)

  return (
    <aside className="detail-pane">
      <div className="detail-header">
        <p>{labelFor(theme, 'themeSubjectAreaSub') || labelFor(theme, 'themeSubjectArea')}</p>
        <h2>{theme.themeTitle || 'Untitled theme'}</h2>
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
          <p>{theme.teacherName && theme.teacherPinyin ? theme.teacherName : theme.deptName_en || theme.deptName}</p>
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

async function fetchThemes({ query, filters, offset, semantic, negativeQuery }) {
  if (semantic && query) {
    const params = new URLSearchParams({ q: query, limit: String(PAGE_SIZE) })
    if (negativeQuery) params.set('negative', negativeQuery)
    const data = await api(`/semantic-search?${params.toString()}`)
    const rows = (data.rows || [])
      .map((row) => ({ ...row.theme, similarity: row.similarity }))
      .filter((theme) => matchesFilters(theme, filters))
    return {
      rows,
      total: rows.length,
    }
  }

  const params = new URLSearchParams({ limit: String(PAGE_SIZE), offset: String(offset) })
  Object.entries(filters).forEach(([key, value]) => {
    if (value) params.set(key, value)
  })
  const path = query ? '/search' : '/themes'
  if (query) params.set('q', query)
  return api(`${path}?${params.toString()}`)
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

function resultSummary(results, semantic) {
  if (semantic) return `${results.rows.length} strongest matches`
  const total = results.total || 0
  return `${total} ${total === 1 ? 'theme' : 'themes'} found`
}

export default App
