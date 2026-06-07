import { useEffect, useState } from "react";

export type ResourceState<T> =
  | { status: "loading" }
  | { status: "ready"; data: T }
  | { status: "error"; message: string };

// useResource runs an async loader on mount and tracks loading/ready/error,
// guarding against state updates after unmount. It replaces the near-identical
// fetch-and-track blocks the page, archive, and admin views each carried.
export function useResource<T>(loader: () => Promise<T>, errorMessage: string): ResourceState<T> {
  const [state, setState] = useState<ResourceState<T>>({ status: "loading" });

  useEffect(() => {
    let cancelled = false;

    loader()
      .then((data) => {
        if (!cancelled) {
          setState({ status: "ready", data });
        }
      })
      .catch(() => {
        if (!cancelled) {
          setState({ status: "error", message: errorMessage });
        }
      });

    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [errorMessage, loader]);

  return state;
}
