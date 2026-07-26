export type MatchStatus = 'SCHEDULED' | 'LIVE' | 'HALF_TIME' | 'PAUSED' | 'FINISHED' | 'CANCELLED' | 'STALE' | 'ERROR';

export type ConfidenceBand = 'HIGH' | 'MEDIUM' | 'LOW';
export type PredictionStatus = 'OK' | 'STALE' | 'DEGRADED' | 'UNAVAILABLE';

export interface ScoreState {
  home: number;
  away: number;
}

export interface CardState {
  home: number;
  away: number;
}

export interface TeamStats {
  shots: number;
  shotsOnTarget: number;
  shotsBlocked: number;
  xg: number;
  corners: number;
  fouls: number;
  dangerousAttacks: number;
}

export interface WindowStats {
  windowSeconds: number;
  home: TeamStats;
  away: TeamStats;
}

export interface MatchState {
  matchId: string;
  status: MatchStatus;
  period: number;
  clockSeconds: number;
  score: ScoreState;
  redCards: CardState;
  yellowCards: CardState;
  substitutions: CardState;
  windows?: Record<number, WindowStats>;
  provider: string;
  providerUpdatedAt?: string;
  receivedAt?: string;
  feedLagMs?: number;
  stateVersion: number;
  homeTeamId: string;
  awayTeamId: string;
  competitionId: string;
  seasonId?: string;
  // Display names. Providers that identify teams by numeric ID send these
  // separately; use the teamLabel/competitionLabel helpers rather than
  // reading them directly, so the ID still shows when a name is absent.
  homeTeamName?: string;
  awayTeamName?: string;
  competitionName?: string;
}

export interface Probabilities {
  goalNext5m: number;
  goalNext10m: number;
  goalBeforeFullTime: number;
  twoOrMoreGoalsBeforeFullTime?: number;
  homeGoalBeforeFullTime?: number;
  awayGoalBeforeFullTime?: number;
}

export interface Prediction {
  matchId: string;
  asOfMatchSecond: number;
  calculatedAt: string;
  probabilities: Probabilities;
  dataQuality: number;
  confidenceBand: ConfidenceBand;
  status: PredictionStatus;
  modelVersion: string;
  calibratorVersion: string;
  featureVersion: string;
  predictionSequence: number;
  expectedGoalsRemaining?: number;
  contributions?: Array<{ name: string; value: number; coefficient: number; contribution: number }>;
}

export interface TrackRecord {
  accuracyPct: number;
  resolvedCount: number;
}

export interface MatchUpdate {
  state: MatchState;
  prediction: Prediction;
  trackRecord: TrackRecord;
  timestamp: string;
}

export interface MatchEvent {
  eventId: string;
  matchId: string;
  provider: string;
  eventType: string;
  teamId?: string;
  matchSecond: number;
  period: number;
  xg?: number;
  receivedAt: string;
}

export interface EvaluationMetrics {
  brierScore: number;
  logLoss: number;
  ece: number;
  hitRatePct: number;
  calibrationCurve: Array<{
    bin: string;
    predicted: number;
    observed: number;
    count: number;
  }>;
  totalSnapshots: number;
  /** Previsões cujo desfecho já é conhecido. Só estas foram medidas. */
  resolvedCount: number;
  /** Partidas distintas por trás das amostras — o tamanho efetivo da amostra,
   *  já que todas as previsões de uma partida dividem um único desfecho. */
  matchCount: number;
  dataQualityAvg: number;
  staleFeedPct: number;
  modelVersion: string;
  featureVersion: string;
  evaluatedPeriod: string;
}
