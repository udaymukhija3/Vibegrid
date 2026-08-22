import clsx from "clsx";
import { Check, Crown, UserRound } from "lucide-react";
import type { VibeCard as VibeCardType } from "@/types/vibe";

export function VibeCard({
  card,
  selected = false,
  selectable = false,
  disabled = false,
  onSelect
}: {
  card: VibeCardType;
  selected?: boolean;
  selectable?: boolean;
  disabled?: boolean;
  onSelect?: () => void;
}) {
  const content = (
    <>
      <div className="flex items-start justify-between gap-3">
        <div>
          <p className="vg-meta text-ink/[.55]">{card.winner ? "Crew pick" : card.isYours ? "Your card" : "Vibe card"}</p>
          <h3 className="mt-2 text-2xl font-black leading-none sm:text-3xl">{card.title}</h3>
        </div>
        {card.winner ? (
          <span className="vg-card-badge bg-lime"><Crown aria-hidden size={17} /> winner</span>
        ) : selected ? (
          <span className="vg-card-badge bg-violet text-cream"><Check aria-hidden size={17} /> backed</span>
        ) : null}
      </div>

      <div className="mt-5 grid grid-cols-2 gap-2">
        {card.tiles.map((tile) => (
          <span key={tile.id} className="vg-card-fragment">{tile.text}</span>
        ))}
      </div>

      {(card.authorName || card.votes !== undefined) && (
        <div className="mt-5 flex items-center justify-between gap-3 border-t-2 border-ink/[.15] pt-3 font-mono text-xs font-bold uppercase tracking-[0.08em]">
          <span className="flex items-center gap-1.5"><UserRound aria-hidden size={15} />{card.authorName ?? "Author hidden"}</span>
          {card.votes !== undefined && <span>{card.votes} {card.votes === 1 ? "vote" : "votes"}</span>}
        </div>
      )}
    </>
  );

  if (selectable) {
    return (
      <button
        type="button"
        onClick={onSelect}
        disabled={disabled}
        aria-pressed={selected}
        className={clsx("vg-vibe-card w-full text-left", selected && "vg-vibe-card-selected", disabled && "opacity-55")}
      >
        {content}
      </button>
    );
  }

  return <article className={clsx("vg-vibe-card", card.winner && "vg-vibe-card-winner")}>{content}</article>;
}
