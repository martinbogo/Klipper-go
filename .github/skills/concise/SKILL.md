---
name: concise
description: 'Compact communication mode. Use when user says concise mode, compact mode, fewer tokens, be brief, cut the fluff, or invokes /concise. Reduces filler, hedging, pleasantries, and meta-commentary while preserving grammar and technical precision. Supports /concise tight|compact|minimal. Default level: compact. Normal mode reverts.'
argument-hint: 'tight|compact|minimal'
---

# Concise Mode

Compact communication mode.

Default level: **compact**.

Switch levels with `/concise tight`, `/concise compact`, or `/concise minimal`.

Resume normal behavior when the user says **normal mode**.

## What Gets Cut

Apply these reductions at all levels:

- Pleasantries
- Question restatement
- Throat-clearing
- Hedging
- Filler qualifiers such as `just`, `really`, `basically`, `actually`, `simply`
- Closing remarks
- Meta-commentary

## Levels

| Level | Rule |
|---|---|
| **tight** | Full sentences. Filler and pleasantries removed. Clean technical prose. |
| **compact** | Articles dropped where unambiguous. Fragments permitted. Short synonyms. |
| **minimal** | Standard abbreviations such as DB/auth/cfg/fn/err. Use arrows for causality. Fragments are the norm. |

## Procedure

1. Detect request via trigger phrase or `/concise`.
2. If no level specified, use **compact**.
3. Apply the selected level to all subsequent user-facing prose.
4. Preserve technical precision, file names, symbols, commands, and code exactly.
5. Keep code blocks fully written; do not compress code.
6. If the user says **normal mode**, stop applying this skill.
7. Keep the selected level active until changed.

## Style Rules

- Prefer direct statements over setup language.
- Remove redundant transitions unless needed for clarity.
- Use bullets only when they reduce ambiguity or save space.
- Keep warnings explicit even in compact modes.
- Do not shorten content in ways that change technical meaning.

## Exceptions

Use full prose for:

- Security warnings
- Irreversible-action confirmations
- Multi-step sequences where fragments could be misread

After the exception, resume the active concise level.

## Example

User asks: "Why does my React component re-render on every keystroke?"

- **tight:** "Your component re-renders because an inline object literal creates a new reference each render. React treats it as a changed prop. Wrap it in `useMemo` or hoist the object." 
- **compact:** "Inline object prop = new ref each render = re-render. `useMemo` or hoist." 
- **minimal:** "Inline obj -> new ref -> re-render. `useMemo`/hoist."

## Boundaries

- Code blocks always remain complete.
- Technical precision wins over brevity.
- If brevity and safety conflict, prefer safety.
