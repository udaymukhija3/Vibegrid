import type { Tile } from "@/types/puzzle";

export type VibeBoard = {
  id: string;
  boardNumber: number;
  publishDate: string;
  prompt: string;
  tiles: Tile[];
};

/** One opponent in the practice round, dealt from the board's own template. */
export type VibeHouseCard = {
  title: string;
  tileIndices: number[];
};

/** A board plus the cards to play it against. Practice endpoints only. */
export type VibePracticeBoard = VibeBoard & {
  houseCards: VibeHouseCard[];
};

export type VibeCard = {
  id: string;
  title: string;
  tiles: Tile[];
  isYours: boolean;
  authorName?: string;
  votes?: number;
  winner?: boolean;
};

export type VibeMake = {
  board: VibeBoard;
  submission?: VibeCard;
  submittedCount: number;
  memberCount: number;
};

export type VibeJudge = {
  board: VibeBoard;
  eligible: boolean;
  hasVoted: boolean;
  yourVoteId?: string;
  cards?: VibeCard[];
  submittedCount: number;
};

export type VibeResult = {
  board: VibeBoard;
  official: boolean;
  /** The crew split its ballots across the top, so no card is crowned. */
  tied: boolean;
  submissionCount: number;
  voteCount: number;
  cards: VibeCard[];
};

export type VibeMember = {
  memberId?: string;
  displayName: string;
  isYou: boolean;
  submittedToday: boolean;
};

export type VibeCrewDaily = {
  crew: {
    inviteCode: string;
    name: string;
    joinPath: string;
    isOwner: boolean;
  };
  isMember: boolean;
  crewStreak: number;
  today: VibeMake;
  judge?: VibeJudge;
  result?: VibeResult;
  members?: VibeMember[];
};
