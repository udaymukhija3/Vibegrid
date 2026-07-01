"use client";

import { useCallback, useEffect, useState, type ReactNode } from "react";
import clsx from "clsx";
import { toast } from "sonner";
import type {
  AdminPuzzle,
  AdminPuzzlePreview,
  AdminQueueHealth,
  ModerationAction,
  ModerationAppeal,
  ModerationReport
} from "@/types/puzzle";
import {
  approveCommunityPuzzle,
  archivePuzzle,
  checkAdminSession,
  createDraftPuzzle,
  fetchAdminPuzzles,
  fetchAnalytics,
  fetchAppeals,
  fetchAuditLog,
  fetchPuzzlePreview,
  fetchQueueHealth,
  fetchReports,
  loginAdmin,
  logoutAdmin,
  publishPuzzle,
  reinstatePuzzle,
  resolveAppeal,
  resolveReport,
  type PuzzleAnalytics
} from "@/lib/adminApi";
import { formatDifficulty, formatOrigin, formatStatus } from "@/lib/displayLabels";
import { formatSeconds } from "@/lib/game";
import { PuzzleDraftForm } from "@/components/PuzzleDraftForm";

const statusStyles: Record<string, string> = {
  PUBLISHED: "bg-mint",
  PENDING: "bg-plum text-white",
  DRAFT: "bg-yolk",
  ARCHIVED: "bg-neutral-200",
  OPEN: "bg-yolk",
  ACTIONED: "bg-mint",
  DISMISSED: "bg-neutral-200",
  RESOLVED: "bg-neutral-200"
};

export function AdminDesk() {
  const [isAuthenticated, setIsAuthenticated] = useState<boolean | null>(null);
  const [passwordInput, setPasswordInput] = useState("");
  const [puzzles, setPuzzles] = useState<AdminPuzzle[] | null>(null);
  const [queueHealth, setQueueHealth] = useState<AdminQueueHealth | null>(null);
  const [reports, setReports] = useState<ModerationReport[]>([]);
  const [appeals, setAppeals] = useState<ModerationAppeal[]>([]);
  const [auditLog, setAuditLog] = useState<ModerationAction[]>([]);
  const [publishDates, setPublishDates] = useState<Record<string, string>>({});
  const [notes, setNotes] = useState<Record<string, string>>({});
  const [analytics, setAnalytics] = useState<Record<string, PuzzleAnalytics>>({});
  const [previews, setPreviews] = useState<Record<string, AdminPuzzlePreview>>({});
  const [openAnalyticsId, setOpenAnalyticsId] = useState<string | null>(null);
  const [openPreviewId, setOpenPreviewId] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);

  const loadPuzzles = useCallback(async () => {
    setError("");
    try {
      setPuzzles(await fetchAdminPuzzles());
    } catch (loadError) {
      const message = loadError instanceof Error ? loadError.message : "Could not load puzzles.";
      setPuzzles(null);
      setError(message);
      toast.error(message);
    }
  }, []);

  const loadModeration = useCallback(async () => {
    try {
      const [nextReports, nextAppeals, nextAuditLog] = await Promise.all([
        fetchReports(),
        fetchAppeals(),
        fetchAuditLog()
      ]);
      setReports(nextReports);
      setAppeals(nextAppeals);
      setAuditLog(nextAuditLog);
    } catch (loadError) {
      const message = loadError instanceof Error ? loadError.message : "Could not load moderation.";
      setError(message);
      toast.error(message);
    }
  }, []);

  const loadQueueHealth = useCallback(async () => {
    try {
      setQueueHealth(await fetchQueueHealth());
    } catch (loadError) {
      const message = loadError instanceof Error ? loadError.message : "Could not load queue health.";
      setQueueHealth(null);
      setError(message);
      toast.error(message);
    }
  }, []);

  const refreshAll = useCallback(async () => {
    await Promise.all([loadPuzzles(), loadModeration(), loadQueueHealth()]);
  }, [loadModeration, loadPuzzles, loadQueueHealth]);

  useEffect(() => {
    checkAdminSession()
      .then(async (ok) => {
        setIsAuthenticated(ok);
        if (ok) {
          await refreshAll();
        }
      })
      .catch(() => setIsAuthenticated(false));
  }, [refreshAll]);

  async function connect() {
    const password = passwordInput.trim();
    if (!password) {
      return;
    }
    setBusy(true);
    setError("");
    try {
      await loginAdmin(password);
      setPasswordInput("");
      setIsAuthenticated(true);
      await refreshAll();
    } catch (loginError) {
      const message = loginError instanceof Error ? loginError.message : "Could not sign in.";
      setError(message);
      toast.error(message);
    } finally {
      setBusy(false);
    }
  }

  async function disconnect() {
    await logoutAdmin().catch(() => undefined);
    setIsAuthenticated(false);
    setPuzzles(null);
    setQueueHealth(null);
    setReports([]);
    setAppeals([]);
    setAuditLog([]);
    setNotice("");
    setError("");
  }

  async function toggleAnalytics(puzzleId: string) {
    if (openAnalyticsId === puzzleId) {
      setOpenAnalyticsId(null);
      return;
    }
    setOpenAnalyticsId(puzzleId);
    if (!analytics[puzzleId]) {
      try {
        const data = await fetchAnalytics(puzzleId);
        setAnalytics((current) => ({ ...current, [puzzleId]: data }));
      } catch (analyticsError) {
        const message = analyticsError instanceof Error ? analyticsError.message : "Could not load analytics.";
        setError(message);
        toast.error(message);
      }
    }
  }

  async function togglePreview(puzzleId: string) {
    if (openPreviewId === puzzleId) {
      setOpenPreviewId(null);
      return;
    }
    setOpenPreviewId(puzzleId);
    if (!previews[puzzleId]) {
      try {
        const data = await fetchPuzzlePreview(puzzleId);
        setPreviews((current) => ({ ...current, [puzzleId]: data }));
      } catch (previewError) {
        const message = previewError instanceof Error ? previewError.message : "Could not load preview.";
        setError(message);
        toast.error(message);
      }
    }
  }

  async function publish(puzzleId: string) {
    const date = publishDates[puzzleId];
    if (!date) {
      setError("Pick a publish date first.");
      return;
    }
    await runAction(`Published for ${date}.`, async () => {
      await publishPuzzle(puzzleId, date);
      await refreshAll();
    });
  }

  async function archive(puzzleId: string) {
    await runAction("Puzzle archived.", async () => {
      await archivePuzzle(puzzleId);
      await refreshAll();
    });
  }

  async function approveCommunity(puzzleId: string) {
    await runAction("Community grid approved and live.", async () => {
      await approveCommunityPuzzle(puzzleId);
      await refreshAll();
    });
  }

  async function reinstate(puzzleId: string) {
    await runAction("Puzzle reinstated.", async () => {
      await reinstatePuzzle(puzzleId);
      await refreshAll();
    });
  }

  async function moderateReport(reportId: string, action: "ARCHIVE" | "DISMISS") {
    await runAction(action === "ARCHIVE" ? "Report actioned." : "Report dismissed.", async () => {
      await resolveReport(reportId, action, notes[reportId] ?? "");
      await refreshAll();
    });
  }

  async function moderateAppeal(appealId: string, action: "REINSTATE" | "CLOSE") {
    await runAction(action === "REINSTATE" ? "Appeal approved." : "Appeal closed.", async () => {
      await resolveAppeal(appealId, action, notes[appealId] ?? "");
      await refreshAll();
    });
  }

  async function runAction(successMessage: string, action: () => Promise<void>) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await action();
      setNotice(successMessage);
      toast.success(successMessage);
    } catch (actionError) {
      const message = actionError instanceof Error ? actionError.message : "Could not complete that action.";
      setError(message);
      toast.error(message);
    } finally {
      setBusy(false);
    }
  }

  if (isAuthenticated === null) {
    return <p className="mt-6 font-semibold text-neutral-600">Checking admin session.</p>;
  }

  if (!isAuthenticated) {
    return (
      <div className="mt-6 max-w-md rounded border-2 border-ink bg-white p-5 shadow-[0_6px_0_#171717]">
        <h2 className="text-lg font-black">Admin sign in</h2>
        <p className="mt-1 text-sm text-neutral-600">Use the admin password to manage puzzles and reports.</p>
        <input
          type="password"
          value={passwordInput}
          onChange={(event) => setPasswordInput(event.target.value)}
          onKeyDown={(event) => event.key === "Enter" && connect()}
          placeholder="admin password"
          className="mt-4 h-11 w-full rounded border-2 border-ink px-3 font-semibold"
        />
        <button
          type="button"
          disabled={busy}
          onClick={connect}
          className="mt-3 inline-flex h-11 items-center justify-center rounded border-2 border-ink bg-mint px-4 font-black shadow-[0_4px_0_#171717] disabled:opacity-50"
        >
          Sign in
        </button>
        {error && <p className="mt-3 text-sm font-bold text-tomato">{error}</p>}
      </div>
    );
  }

  return (
    <div className="mt-6 grid gap-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div aria-live="polite" className="min-h-6 text-sm font-bold">
          {error && <span className="text-tomato">{error}</span>}
          {!error && notice && <span className="text-plum">{notice}</span>}
        </div>
        <button
          type="button"
          onClick={disconnect}
          className="inline-flex h-9 items-center rounded border border-neutral-300 bg-white px-3 text-sm font-black text-neutral-700"
        >
          Sign out
        </button>
      </div>

      <QueueHealthPanel queueHealth={queueHealth} />

      <ModerationPanel
        reports={reports}
        appeals={appeals}
        auditLog={auditLog}
        notes={notes}
        setNote={(id, note) => setNotes((current) => ({ ...current, [id]: note }))}
        busy={busy}
        onReport={moderateReport}
        onAppeal={moderateAppeal}
      />

      <section className="rounded border-2 border-ink bg-white p-4 shadow-[0_6px_0_#171717]">
        <h2 className="text-lg font-black">Pipeline</h2>
        {!puzzles && <p className="mt-3 font-semibold text-neutral-600">Loading puzzles.</p>}
        {puzzles && puzzles.length === 0 && (
          <p className="mt-3 font-semibold text-neutral-600">No puzzles yet. Create the first draft below.</p>
        )}
        <div className="mt-3 divide-y divide-neutral-200">
          {puzzles?.map((puzzle) => (
            <div key={puzzle.id} className="py-4">
              <div className="grid gap-3 sm:grid-cols-[auto_1fr_auto] sm:items-center">
                <span className="font-black">#{puzzle.puzzleNumber}</span>
                <div>
                  <p className="font-black">{puzzle.groups.map((group) => group.name).join(" · ")}</p>
                  <p className="text-sm text-neutral-600">
                    {formatDifficulty(puzzle.difficulty)} · {formatOrigin(puzzle.origin)}
                    {puzzle.publishDate ? ` · ${puzzle.publishDate}` : ""}
                  </p>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <StatusBadge status={formatStatus(puzzle.status)} rawStatus={puzzle.status} />
                  <button
                    type="button"
                    onClick={() => togglePreview(puzzle.id)}
                    aria-expanded={openPreviewId === puzzle.id}
                    className="inline-flex h-9 items-center rounded border border-neutral-300 bg-white px-3 text-sm font-black text-neutral-700"
                  >
                    Preview
                  </button>
                  <button
                    type="button"
                    onClick={() => toggleAnalytics(puzzle.id)}
                    aria-expanded={openAnalyticsId === puzzle.id}
                    className="inline-flex h-9 items-center rounded border border-neutral-300 bg-white px-3 text-sm font-black text-neutral-700"
                  >
                    Stats
                  </button>
                  {puzzle.status === "DRAFT" && (
                    <>
                      <input
                        type="date"
                        value={publishDates[puzzle.id] ?? ""}
                        onChange={(event) =>
                          setPublishDates((current) => ({ ...current, [puzzle.id]: event.target.value }))
                        }
                        className="h-9 rounded border-2 border-ink px-2 text-sm font-semibold"
                      />
                      <button
                        type="button"
                        disabled={busy}
                        onClick={() => publish(puzzle.id)}
                        className="inline-flex h-9 items-center rounded border-2 border-ink bg-mint px-3 text-sm font-black shadow-[0_3px_0_#171717] disabled:opacity-50"
                      >
                        Publish
                      </button>
                    </>
                  )}
                  {puzzle.status === "PENDING" && puzzle.origin === "COMMUNITY" && (
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() => approveCommunity(puzzle.id)}
                      className="inline-flex h-9 items-center rounded border-2 border-ink bg-mint px-3 text-sm font-black shadow-[0_3px_0_#171717] disabled:opacity-50"
                    >
                      Approve
                    </button>
                  )}
                  {puzzle.status === "PUBLISHED" && (
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() => archive(puzzle.id)}
                      className="inline-flex h-9 items-center rounded border border-neutral-300 bg-white px-3 text-sm font-black text-neutral-700 disabled:opacity-50"
                    >
                      Archive
                    </button>
                  )}
                  {puzzle.status === "ARCHIVED" && (
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() => reinstate(puzzle.id)}
                      className="inline-flex h-9 items-center rounded border border-neutral-300 bg-white px-3 text-sm font-black text-neutral-700 disabled:opacity-50"
                    >
                      Reinstate
                    </button>
                  )}
                </div>
              </div>

              {openPreviewId === puzzle.id && <PreviewPanel data={previews[puzzle.id]} />}
              {openAnalyticsId === puzzle.id && <AnalyticsPanel data={analytics[puzzle.id]} />}
            </div>
          ))}
        </div>
      </section>

      <section className="rounded border-2 border-ink bg-white p-4 shadow-[0_6px_0_#171717]">
        <PuzzleDraftForm
          submitLabel="Save draft"
          onSubmit={async (input) => {
            const created = await createDraftPuzzle(input);
            setNotice(`Draft #${created.puzzleNumber} saved.`);
            setError("");
            toast.success(`Draft #${created.puzzleNumber} saved.`);
            await refreshAll();
          }}
        />
      </section>
    </div>
  );
}

const queueCoverageStyles: Record<string, string> = {
  EDITORIAL: "border-mint bg-mint/20",
  EVERGREEN: "border-yolk bg-yolk/20",
  UNSCHEDULED: "border-tomato bg-tomato/10"
};

function QueueHealthPanel({ queueHealth }: { queueHealth: AdminQueueHealth | null }) {
  if (!queueHealth) {
    return (
      <section className="rounded border-2 border-ink bg-white p-4 shadow-[0_6px_0_#171717]">
        <h2 className="text-lg font-black">Queue health</h2>
        <p className="mt-3 font-semibold text-neutral-600">Loading launch calendar.</p>
      </section>
    );
  }

  const unscheduled = queueHealth.days.filter((day) => day.coverage === "UNSCHEDULED").length;

  return (
    <section className="rounded border-2 border-ink bg-white p-4 shadow-[0_6px_0_#171717]">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-xs font-black text-plum">Launch calendar</p>
          <h2 className="text-lg font-black">Queue health</h2>
          <p className="mt-1 text-sm font-semibold text-neutral-600">
            Server today is {queueHealth.today}. Editorial puzzles win; evergreen fallback fills open days.
          </p>
        </div>
        <div className="grid grid-cols-2 gap-2 text-sm font-bold sm:grid-cols-4">
          <QueueStat label="Scheduled" value={queueHealth.scheduledEditorial} />
          <QueueStat label="Fallback" value={queueHealth.evergreenFallbacks} />
          <QueueStat label="Drafts" value={queueHealth.drafts} />
          <QueueStat label="Pending UGC" value={queueHealth.pendingCommunity} />
        </div>
      </div>

      {unscheduled > 0 && (
        <p className="mt-3 rounded border border-tomato bg-tomato/10 px-3 py-2 text-sm font-bold text-ink">
          {unscheduled} day{unscheduled === 1 ? "" : "s"} have no editorial puzzle and no evergreen fallback. Add bank
          content before launch.
        </p>
      )}

      <div className="mt-4 grid gap-2 sm:grid-cols-2 lg:grid-cols-7">
        {queueHealth.days.map((day) => (
          <article
            key={day.date}
            className={clsx("rounded border-2 p-3", queueCoverageStyles[day.coverage] ?? "border-neutral-200 bg-neutral-50")}
          >
            <p className="text-xs font-black text-neutral-500">{day.date}</p>
            <p className="mt-1 font-black">{queueCoverageLabel(day.coverage)}</p>
            {day.puzzleNumber !== undefined && (
              <p className="mt-1 text-sm font-semibold text-neutral-600">VibeGrid #{day.puzzleNumber}</p>
            )}
          </article>
        ))}
      </div>
    </section>
  );
}

function QueueStat({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded border border-neutral-200 bg-neutral-50 px-3 py-2">
      <p className="text-lg font-black">{value}</p>
      <p className="text-xs text-neutral-500">{label}</p>
    </div>
  );
}

function queueCoverageLabel(coverage: AdminQueueHealth["days"][number]["coverage"]) {
  switch (coverage) {
    case "EDITORIAL":
      return "Editorial";
    case "EVERGREEN":
      return "Evergreen";
    case "UNSCHEDULED":
    default:
      return "Unscheduled";
  }
}

function ModerationPanel({
  reports,
  appeals,
  auditLog,
  notes,
  setNote,
  busy,
  onReport,
  onAppeal
}: {
  reports: ModerationReport[];
  appeals: ModerationAppeal[];
  auditLog: ModerationAction[];
  notes: Record<string, string>;
  setNote: (id: string, note: string) => void;
  busy: boolean;
  onReport: (id: string, action: "ARCHIVE" | "DISMISS") => Promise<void>;
  onAppeal: (id: string, action: "REINSTATE" | "CLOSE") => Promise<void>;
}) {
  return (
    <section className="rounded border-2 border-ink bg-white p-4 shadow-[0_6px_0_#171717]">
      <h2 className="text-lg font-black">Moderation</h2>

      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        <QueueList title="Reports">
          {reports.length === 0 ? (
            <p className="text-sm font-semibold text-neutral-600">No reports yet.</p>
          ) : (
            reports.map((report) => (
              <ModerationCard key={report.id} status={report.status}>
                <div className="flex flex-wrap items-start justify-between gap-2">
                  <div>
                    <p className="font-black">VibeGrid #{report.puzzleNumber}</p>
                    <p className="text-sm font-semibold text-neutral-600">
                      {formatOrigin(report.puzzleOrigin)} · {report.reason.replace("_", " ").toLowerCase()}
                    </p>
                  </div>
                  <StatusBadge status={report.status === "OPEN" ? "Open" : report.status.toLowerCase()} rawStatus={report.status} />
                </div>
                {report.details && <p className="mt-2 text-sm text-neutral-700">{report.details}</p>}
                {report.contact && <p className="mt-1 text-xs font-semibold text-neutral-500">{report.contact}</p>}
                {report.status === "OPEN" && (
                  <ActionControls
                    id={report.id}
                    note={notes[report.id] ?? ""}
                    setNote={setNote}
                    busy={busy}
                    primaryLabel="Archive"
                    secondaryLabel="Dismiss"
                    onPrimary={() => onReport(report.id, "ARCHIVE")}
                    onSecondary={() => onReport(report.id, "DISMISS")}
                  />
                )}
              </ModerationCard>
            ))
          )}
        </QueueList>

        <QueueList title="Appeals">
          {appeals.length === 0 ? (
            <p className="text-sm font-semibold text-neutral-600">No appeals yet.</p>
          ) : (
            appeals.map((appeal) => (
              <ModerationCard key={appeal.id} status={appeal.status}>
                <div className="flex flex-wrap items-start justify-between gap-2">
                  <div>
                    <p className="font-black">VibeGrid #{appeal.puzzleNumber}</p>
                    <p className="text-sm font-semibold text-neutral-600">
                      {formatOrigin(appeal.puzzleOrigin)} · {formatStatus(appeal.puzzleStatus)}
                    </p>
                  </div>
                  <StatusBadge status={appeal.status === "OPEN" ? "Open" : "Resolved"} rawStatus={appeal.status} />
                </div>
                <p className="mt-2 text-sm text-neutral-700">{appeal.message}</p>
                {appeal.contact && <p className="mt-1 text-xs font-semibold text-neutral-500">{appeal.contact}</p>}
                {appeal.status === "OPEN" && (
                  <ActionControls
                    id={appeal.id}
                    note={notes[appeal.id] ?? ""}
                    setNote={setNote}
                    busy={busy}
                    primaryLabel="Reinstate"
                    secondaryLabel="Close"
                    onPrimary={() => onAppeal(appeal.id, "REINSTATE")}
                    onSecondary={() => onAppeal(appeal.id, "CLOSE")}
                  />
                )}
              </ModerationCard>
            ))
          )}
        </QueueList>
      </div>

      <div className="mt-4 rounded border border-neutral-200 p-3">
        <p className="text-xs font-black text-neutral-500">Audit log</p>
        {auditLog.length === 0 ? (
          <p className="mt-2 text-sm font-semibold text-neutral-600">No moderation actions yet.</p>
        ) : (
          <ul className="mt-2 grid gap-2">
            {auditLog.slice(0, 8).map((action) => (
              <li key={action.id} className="text-sm">
                <span className="font-black">{action.action.replaceAll("_", " ").toLowerCase()}</span>
                {action.puzzleNumber ? ` · #${action.puzzleNumber}` : ""}
                <span className="text-neutral-500"> · {new Date(action.createdAt).toLocaleString()}</span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}

function QueueList({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div>
      <p className="text-xs font-black text-neutral-500">{title}</p>
      <div className="mt-2 grid gap-3">{children}</div>
    </div>
  );
}

function ModerationCard({ children, status }: { children: ReactNode; status: string }) {
  return (
    <article
      className={clsx(
        "rounded border p-3",
        status === "OPEN" ? "border-ink bg-yolk/10" : "border-neutral-200 bg-neutral-50"
      )}
    >
      {children}
    </article>
  );
}

function ActionControls({
  id,
  note,
  setNote,
  busy,
  primaryLabel,
  secondaryLabel,
  onPrimary,
  onSecondary
}: {
  id: string;
  note: string;
  setNote: (id: string, note: string) => void;
  busy: boolean;
  primaryLabel: string;
  secondaryLabel: string;
  onPrimary: () => void;
  onSecondary: () => void;
}) {
  return (
    <div className="mt-3 grid gap-2">
      <input
        value={note}
        onChange={(event) => setNote(id, event.target.value)}
        placeholder="Decision note"
        className="h-9 rounded border border-neutral-300 px-2 text-sm"
      />
      <div className="flex flex-wrap gap-2">
        <button
          type="button"
          disabled={busy}
          onClick={onPrimary}
          className="inline-flex h-9 items-center rounded border-2 border-ink bg-mint px-3 text-sm font-black shadow-[0_3px_0_#171717] disabled:opacity-50"
        >
          {primaryLabel}
        </button>
        <button
          type="button"
          disabled={busy}
          onClick={onSecondary}
          className="inline-flex h-9 items-center rounded border border-neutral-300 bg-white px-3 text-sm font-black text-neutral-700 disabled:opacity-50"
        >
          {secondaryLabel}
        </button>
      </div>
    </div>
  );
}

function StatusBadge({ status, rawStatus }: { status: string; rawStatus: string }) {
  return (
    <span className={clsx("rounded px-3 py-1 text-xs font-black", statusStyles[rawStatus] ?? "bg-neutral-200")}>
      {status}
    </span>
  );
}

const previewColorClasses = ["bg-mint", "bg-yolk", "bg-tomato text-white", "bg-plum text-white"];

function previewColorClass(colorIndex: number) {
  const index = ((colorIndex % previewColorClasses.length) + previewColorClasses.length) % previewColorClasses.length;
  return previewColorClasses[index];
}

function PreviewPanel({ data }: { data: AdminPuzzlePreview | undefined }) {
  if (!data) {
    return <p className="mt-3 text-sm font-semibold text-neutral-600">Loading preview...</p>;
  }

  const groupByTile = new Map<string, { name: string; colorIndex: number }>();
  for (const group of data.groups) {
    for (const tileId of group.tileIds) {
      groupByTile.set(tileId, { name: group.name, colorIndex: group.colorIndex });
    }
  }

  return (
    <div className="mt-3 grid gap-4 rounded border-2 border-ink bg-neutral-50 p-3">
      <div>
        <p className="text-xs font-black text-neutral-500">Public board preview</p>
        <p className="mt-1 text-sm font-semibold text-neutral-600">
          This is the same tile order players will see after publish or approval.
        </p>
        <div className="mt-3 grid grid-cols-4 gap-2">
          {data.puzzle.tiles.map((tile) => {
            const owner = groupByTile.get(tile.id);
            return (
              <div
                key={tile.id}
                title={owner?.name}
                className={clsx(
                  "flex min-h-14 items-center justify-center rounded border-2 border-ink px-2 text-center text-xs font-black sm:text-sm",
                  owner ? previewColorClass(owner.colorIndex) : "bg-white"
                )}
              >
                {tile.text}
              </div>
            );
          })}
        </div>
      </div>

      <div>
        <p className="text-xs font-black text-neutral-500">Answer key</p>
        <div className="mt-2 grid gap-2 md:grid-cols-2">
          {data.groups
            .slice()
            .sort((left, right) => left.colorIndex - right.colorIndex)
            .map((group) => (
              <article key={group.id} className="rounded border border-neutral-200 bg-white p-3">
                <div className="flex items-center gap-2">
                  <span
                    aria-hidden
                    className={clsx(
                      "h-3 w-3 rounded-full border border-ink",
                      previewColorClass(group.colorIndex)
                    )}
                  />
                  <p className="font-black">{group.name}</p>
                </div>
                <p className="mt-1 text-sm font-semibold text-neutral-600">{group.explanation}</p>
                <p className="mt-2 text-sm text-neutral-700">{group.tiles.map((tile) => tile.text).join(", ")}</p>
              </article>
            ))}
        </div>
      </div>
    </div>
  );
}

function AnalyticsPanel({ data }: { data: PuzzleAnalytics | undefined }) {
  if (!data) {
    return <p className="mt-3 text-sm font-semibold text-neutral-600">Loading analytics...</p>;
  }

  const { stats, wrongGuesses } = data;

  if (stats.players === 0) {
    return <p className="mt-3 text-sm font-semibold text-neutral-600">No plays yet.</p>;
  }

  return (
    <div className="mt-3 rounded border-2 border-ink bg-neutral-50 p-3">
      <div className="grid grid-cols-2 gap-2 text-sm font-semibold sm:grid-cols-4">
        <p>{stats.players} {stats.players === 1 ? "player" : "players"}</p>
        <p>{Math.round(stats.solveRate * 100)}% solved</p>
        <p>~{stats.medianMistakes.toFixed(1)} mistakes</p>
        {stats.medianSolveSeconds !== undefined && <p>~{formatSeconds(stats.medianSolveSeconds)} median</p>}
      </div>

      <p className="mt-3 text-xs font-black text-neutral-500">Most common wrong guesses</p>
      {wrongGuesses.length === 0 ? (
        <p className="mt-1 text-sm text-neutral-500">No wrong guesses recorded yet.</p>
      ) : (
        <ul className="mt-2 grid gap-1">
          {wrongGuesses.map((wrong, index) => (
            <li key={index} className="flex items-center justify-between gap-3 text-sm">
              <span className="font-semibold">{wrong.tiles.join(", ")}</span>
              <span className="font-black text-tomato">×{wrong.count}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
