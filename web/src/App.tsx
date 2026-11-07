import { useEffect, useMemo, useState } from 'react'
import NavBar from './NavBar'
import Rektion from './Rektion'

type Level = 'A1' | 'A2' | 'B1' | 'B2' | 'C1'
type Article = 'der' | 'die' | 'das'
type Word = { id: number; noun: string; article: Article; level: Level; definition: string; translations: string[]; translationSource: string; sourceUrl: string }
type Feedback = { correct: boolean; article: Article }
const levels: Level[] = ['A1', 'A2', 'B1', 'B2', 'C1']
const articles: Article[] = ['der', 'die', 'das']

export default function App() {
  if (window.location.pathname.startsWith('/rektion')) return <Rektion />
  const [level, setLevel] = useState<Level>('A1')
  const [word, setWord] = useState<Word | null>(null)
  const [feedback, setFeedback] = useState<Feedback | null>(null)
  const [selected, setSelected] = useState<Article | null>(null)
  const [seen, setSeen] = useState<number[]>([])
  const [score, setScore] = useState({ correct: 0, answered: 0 })
  const [missedWords, setMissedWords] = useState<Word[]>([])
  const [reviewOpen, setReviewOpen] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  async function nextWord(nextLevel = level, excluded = seen) {
    setLoading(true); setError(''); setFeedback(null); setSelected(null)
    try {
      const query = new URLSearchParams({ level: nextLevel, exclude: excluded.join(',') })
      const response = await fetch(`/api/words/random?${query}`)
      if (!response.ok) throw new Error(response.status === 404 ? 'No words are available for this level yet.' : 'Could not load a word. Is the Go API running?')
      const next: Word = await response.json()
      setWord(next); setSeen((old) => [...old, next.id])
    } catch (e) { setWord(null); setError(e instanceof Error ? e.message : 'Something went wrong.') }
    finally { setLoading(false) }
  }

  useEffect(() => { void nextWord(level, []) }, [level])

  async function answer(article: Article) {
    if (!word || feedback) return
    setSelected(article)
    try {
      const response = await fetch('/api/answer', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ id: word.id, article }) })
      if (!response.ok) throw new Error('Could not check this answer.')
      const result: Feedback = await response.json()
      setFeedback(result)
      if (!result.correct) setMissedWords((old) => [...old, word])
      setScore((old) => ({ correct: old.correct + (result.correct ? 1 : 0), answered: old.answered + 1 }))
    } catch (e) { setError(e instanceof Error ? e.message : 'Something went wrong.'); setSelected(null) }
  }

  const accuracy = useMemo(() => score.answered ? Math.round((score.correct / score.answered) * 100) : 0, [score])

  return <main className="page-shell">
    <NavBar />
    <section className="layout">
      <div className="level-wrap"><span className="field-label">CHOOSE YOUR LEVEL</span><div className="levels" role="group" aria-label="Choose German level">{levels.map((item) => <button key={item} className={`level ${level === item ? 'active' : ''}`} aria-pressed={level === item} onClick={() => { setLevel(item); setSeen([]); setScore({ correct: 0, answered: 0 }) }}>{item}</button>)}</div><button className="review-toggle" aria-expanded={reviewOpen} aria-controls="mistakes-panel" onClick={() => setReviewOpen((open) => !open)}>Mistakes <span className="mistake-count">{missedWords.length}</span></button></div>
      <section className="quiz-card" aria-live="polite">
        <div className="card-top"><span className="round-label"><span className="status-dot" /> {level} &middot; {level === 'B2' || level === 'C1' ? 'ESTIMATED' : 'PRACTICE'}</span><span className="score-label">SCORE <b>{score.correct}<i>/</i>{score.answered}</b></span></div>
        <div className="question"><span className="prompt">WHICH ARTICLE BELONGS TO</span><h2>{loading ? <span className="skeleton">&nbsp;</span> : word?.noun ?? (error ? 'Ready?' : '—')}</h2>{word && !loading && <div className="word-meaning"><p><span>DEFINITION</span>{word.definition}</p><p><span>{word.translationSource === 'machine' ? 'MACHINE TRANSLATION' : 'ENGLISH'}</span>{word.translations.length ? word.translations.join(', ') : 'No translation linked to this meaning'}</p></div>}<span className="noun-caption">Choose the correct article</span></div>
        <div className="choices" role="group" aria-label="Choose the grammatical article">{articles.map((article) => {
          const isCorrect = feedback?.article === article
          const isWrong = selected === article && feedback && !feedback.correct
          return <button key={article} className={`choice ${selected === article ? 'chosen' : ''} ${feedback && isCorrect ? 'correct' : ''} ${isWrong ? 'wrong' : ''}`} disabled={!word || loading || !!feedback} onClick={() => void answer(article)}><span className="choice-article">{article}</span><span className="choice-icon">{feedback && isCorrect ? '✓' : isWrong ? '×' : '↗'}</span></button>
        })}</div>
        <div className={`feedback ${feedback ? feedback.correct ? 'feedback-good' : 'feedback-bad' : ''}`}>
          {error ? <span>{error}</span> : feedback ? <><span className="feedback-icon">{feedback.correct ? '✓' : '↗'}</span><span>{feedback.correct ? 'Lovely. You got it.' : <>Not quite — it’s <strong>{feedback.article} {word?.noun}</strong>.</>}</span></> : word && <a className="hint" href={word.sourceUrl} target="_blank" rel="noreferrer">Check “{word.noun}” in German Wiktionary →</a>}
        </div>
        <div className="card-bottom"><span>{score.answered ? `${accuracy}% accuracy this session` : 'A fresh start, every time'}</span>{feedback && <button className="next-button" onClick={() => void nextWord()} disabled={loading}>Next word <span>→</span></button>}</div>
      </section>

    </section>
    {reviewOpen && <><button className="drawer-backdrop" aria-label="Close mistakes list" onClick={() => setReviewOpen(false)} /><aside id="mistakes-panel" className="mistakes-drawer" aria-label="Words answered incorrectly"><div className="drawer-header"><div><span className="field-label">REVIEW</span><h2>Missed words</h2></div><button className="drawer-close" aria-label="Close mistakes list" onClick={() => setReviewOpen(false)}>×</button></div>{missedWords.length ? <ul className="mistake-list">{missedWords.map((missed, index) => <li key={`${missed.id}-${index}`}><span className="mistake-article">{missed.article}</span><span className="mistake-word">{missed.noun}</span></li>)}</ul> : <p className="empty-mistakes">No missed words yet. Words you answer incorrectly will appear here.</p>}</aside></>}
  </main>
}
