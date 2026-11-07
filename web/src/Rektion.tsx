import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import NavBar from './NavBar'

type Level = 'A2' | 'B1' | 'B2'
type ExerciseType = 'preposition' | 'case'
type Exercise = { id: string; type: ExerciseType; level: Level; prompt: string }
type Result = { correct: boolean; correctAnswer: string; example: string }
const levels: Level[] = ['A2', 'B1', 'B2']
const types: { id: ExerciseType; label: string }[] = [{ id: 'preposition', label: 'Preposition' }, { id: 'case', label: 'Case' }]
const cases = ['Akkusativ', 'Dativ', 'Genitiv']

export default function Rektion() {
  const [level, setLevel] = useState<Level>('A2')
  const [type, setType] = useState<ExerciseType>('preposition')
  const [exercise, setExercise] = useState<Exercise | null>(null)
  const [answer, setAnswer] = useState('')
  const [result, setResult] = useState<Result | null>(null)
  const [score, setScore] = useState({ correct: 0, total: 0 })
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  async function loadExercise(nextLevel = level, nextType = type) {
    setLoading(true); setError(''); setResult(null); setAnswer('')
    try {
      const query = new URLSearchParams({ level: nextLevel, type: nextType })
      const response = await fetch(`/api/rektion/exercise?${query}`)
      if (!response.ok) throw new Error(response.status === 404 ? `No Rektion entries are available for ${nextLevel}.` : 'Could not load an exercise. Is the Go API running?')
      setExercise(await response.json())
    } catch (e) { setExercise(null); setError(e instanceof Error ? e.message : 'Something went wrong.') }
    finally { setLoading(false) }
  }

  useEffect(() => { void loadExercise(level, type) }, [level, type])

  async function submit(event?: FormEvent) {
    event?.preventDefault()
    if (result) { void loadExercise(); return }
    if (!exercise || !answer.trim() || loading) return
    setLoading(true); setError('')
    try {
      const response = await fetch('/api/rektion/answer', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ id: exercise.id, type, answer }) })
      if (!response.ok) throw new Error('Could not check this answer.')
      const checked: Result = await response.json()
      setResult(checked); setScore(old => ({ correct: old.correct + (checked.correct ? 1 : 0), total: old.total + 1 }))
    } catch (e) { setError(e instanceof Error ? e.message : 'Something went wrong.') }
    finally { setLoading(false) }
  }

  function choose(value: string) { if (!result) setAnswer(value) }

  return <main className="page-shell">
    <NavBar />
    <section className="rektion-layout">
      <div className="rektion-heading"><span className="eyebrow"><span className="eyebrow-line" /> VERB + PREPOSITION</span><h1>Learn the <em>pattern.</em></h1><p>Practice which preposition and case belong with common German verbs.</p></div>
      <section className="rektion-card" aria-live="polite">
        <div className="rektion-controls">
          <label><span className="field-label">LEVEL</span><select value={level} onChange={e => setLevel(e.target.value as Level)}>{levels.map(item => <option key={item}>{item}</option>)}</select></label>
          <label><span className="field-label">EXERCISE</span><select value={type} onChange={e => setType(e.target.value as ExerciseType)}>{types.map(item => <option key={item.id} value={item.id}>{item.label}</option>)}</select></label>
          <span className="rektion-score">SCORE <strong>{score.correct}/{score.total}</strong></span>
        </div>
        <div className="rektion-question"><span className="prompt">{type === 'preposition' ? 'FILL IN THE PREPOSITION' : 'WHICH CASE DOES IT TAKE?'}</span><h2>{loading && !exercise ? 'Loading…' : exercise?.prompt ?? 'Ready?'}</h2>
          {exercise && <form onSubmit={submit}>
            {type === 'preposition' && <input className="rektion-answer" autoComplete="off" aria-label="Preposition" placeholder="Type the preposition" value={answer} onChange={e => setAnswer(e.target.value)} disabled={!!result || loading} />}
            {type === 'case' && <div className="rektion-options" role="group" aria-label="Choose the grammatical case">{cases.map(item => <button type="button" key={item} className={`rektion-option ${answer === item ? 'selected' : ''} ${result && result.correctAnswer === item ? 'correct' : ''} ${result && answer === item && !result.correct ? 'wrong' : ''}`} disabled={!!result || loading} onClick={() => choose(item)}>{item}</button>)}</div>}
            <button className="rektion-submit" type="submit" disabled={loading || (!result && !answer.trim())}>{result ? 'Next' : loading ? 'Checking…' : 'Check'}</button>
          </form>}
        </div>
        <div className={`rektion-feedback ${result ? result.correct ? 'feedback-good' : 'feedback-bad' : ''}`} role="status">
          {error ? error : result && <><strong>{result.correct ? 'Correct!' : `Not quite — ${result.correctAnswer}.`}</strong><span className="rektion-example">{result.example}</span></>}
        </div>
        <div className="rektion-bottom"><span>{score.total ? `${Math.round(score.correct / score.total * 100)}% accuracy this session` : 'Choose an answer to begin.'}</span>{result && <span>Press Enter for next</span>}</div>
      </section>
    </section>
  </main>
}
