import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"
import type { MatchState } from "@/types"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

// Display labels for a match. Real providers identify teams and competitions
// by opaque numeric IDs and send the readable name alongside, so prefer the
// name and fall back to the ID — the mock feed uses names as IDs directly,
// and an ID is still more useful than an empty row.
export function homeLabel(state: MatchState): string {
  return state.homeTeamName || state.homeTeamId
}

export function awayLabel(state: MatchState): string {
  return state.awayTeamName || state.awayTeamId
}

export function competitionLabel(state: MatchState): string {
  return state.competitionName || state.competitionId
}
