export type Difficulty = "EASY" | "MEDIUM" | "HARD";
export type PuzzleStatus = "DRAFT" | "PUBLISHED" | "ARCHIVED";

export type Tile = {
  id: string;
  text: string;
};

export type PublicPuzzle = {
  id: string;
  puzzleNumber: number;
  publishDate: string;
  difficulty: Difficulty;
  tiles: Tile[];
  groupCount: number;
  mistakesAllowed: number;
};

export type SolvedGroup = {
  id: string;
  name: string;
  explanation: string;
  colorIndex: number;
  tileIds: string[];
  tiles: Tile[];
};

export type AdminGroup = {
  id: string;
  name: string;
  explanation: string;
  colorIndex: number;
  tiles: Tile[];
};

export type AdminPuzzle = {
  id: string;
  puzzleNumber: number;
  publishDate: string;
  status: PuzzleStatus;
  difficulty: Difficulty;
  groups: AdminGroup[];
};

export type DraftGroupInput = {
  name: string;
  explanation: string;
  tiles: string[];
};

export type DraftPuzzleInput = {
  difficulty: Difficulty;
  groups: DraftGroupInput[];
};

export type AttemptSnapshot = {
  puzzleId: string;
  solvedGroups: SolvedGroup[];
  revealedGroups: SolvedGroup[];
  mistakes: number;
  guessCount: number;
  startedAt: string;
  completedAt?: string;
  failed: boolean;
  completed: boolean;
};

export type GuessResponse =
  | {
      ok: true;
      isCorrect: true;
      group: SolvedGroup;
      attempt: AttemptSnapshot;
      sessionId: string;
    }
  | {
      ok: true;
      isCorrect: false;
      sessionId: string;
      attempt: AttemptSnapshot;
      revealedGroups?: SolvedGroup[];
    }
  | {
      ok: false;
      error: string;
    };
