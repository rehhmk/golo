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
}

export interface Probabilities {
  goalNext5m: number;
  goalNext10m: number;
  goalBeforeFullTime: number;
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
  dataQualityAvg: number;
  staleFeedPct: number;
  modelVersion: string;
  featureVersion: string;
  evaluatedPeriod: string;
}
