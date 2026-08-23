export interface SingleFlight {
  request: () => Promise<void>;
  stop: () => void;
}

export function createSingleFlight(run: () => Promise<void>): SingleFlight {
  let current: Promise<void> | null = null;
  let pending = false;
  let stopped = false;

  const request = (): Promise<void> => {
    if (stopped) return Promise.resolve();
    if (current) {
      pending = true;
      return current;
    }
    current = (async () => {
      try {
        do {
          pending = false;
          await run();
        } while (pending && !stopped);
      } finally {
        current = null;
      }
    })();
    return current;
  };

  return {
    request,
    stop: () => {
      stopped = true;
      pending = false;
    },
  };
}
