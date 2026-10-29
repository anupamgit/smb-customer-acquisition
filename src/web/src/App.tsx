import { useCallback, useEffect, useState } from "react";
import { affinityForCandidate, MatchChain } from "./MatchChain";
import { WaitStepList } from "./WaitStepList";
import { productWaitLine } from "./waitProgress";
import { useWaitStepDisplay } from "./useWaitStepDisplay";

type DemoBusiness = {
  id: string;
  label: string;
  businessName: string;
  locality: string;
  sampleProducts?: string[];
};

type ConfigState = {
  businessName?: string;
  businessLocality?: string;
  sampleProducts?: string[];
  demoBusinesses?: DemoBusiness[];
  showMatchChain?: boolean;
  missing: string[];
};

type PlaceContact = {
  address?: string;
  phone?: string;
  website?: string;
};

type Opportunity = {
  candidateId?: string;
  placeName?: string;
  placeType?: string;
  contact?: PlaceContact;
  action: string;
  whyFits: string;
  nextAction: string;
};

type Resolution = { inputName: string; resolved: boolean; tagUrn?: string };

type QlooEvidenceRow = { id: string; name: string; affinity: number };

type ScoredCustomer = { customerId?: string; score?: number };

type Run = {
  id: string;
  status: string;
  stage?: string;
  error?: string;
  cohort?: ScoredCustomer[];
  signal?: { boughtNames?: string[] };
  resolutions?: Resolution[];
  qlooEvidence?: QlooEvidenceRow[];
  crossDomainCategories?: string[];
  opportunities?: Opportunity[];
};

const MAX_WAIT_MS = 3 * 60 * 1000 + 15000;
const INITIAL_OPPORTUNITIES_VISIBLE = 3;
const MAX_OPPORTUNITIES_VISIBLE = 5;

async function fetchJson<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, init);
  const body = (await response.json().catch(() => ({}))) as T & { error?: string };
  if (!response.ok) {
    throw new Error(body.error ?? `request failed: ${response.status}`);
  }
  return body as T;
}

function phoneText(raw: string): string {
  const digits = raw.replace(/\D/g, "");
  if (digits.length === 11 && digits.startsWith("1")) {
    return `+1 ${digits.slice(1, 4)} ${digits.slice(4, 7)} ${digits.slice(7)}`;
  }
  return raw;
}

function businessKicker(config: ConfigState): string {
  const name = config.businessName?.trim() || "Your business";
  const where = config.businessLocality?.trim();
  return where ? `${name} · ${where}` : name;
}

function businessName(config: ConfigState): string {
  return config.businessName?.trim() || "Your business";
}

function demoSubtitle(b: DemoBusiness): string {
  const type = b.label?.trim();
  const where = b.locality?.trim();
  if (type && where) {
    return `${type} · ${where}`;
  }
  return type || where || "";
}

function ContactBlock({ contact }: { contact: PlaceContact }) {
  const has = contact.address || contact.phone || contact.website;
  if (!has) return null;
  return (
    <details className="contact">
      <summary className="contact-toggle">Contact</summary>
      <div className="contact-body">
        {contact.address ? (
          <a
            href={`https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(contact.address)}`}
            target="_blank"
            rel="noreferrer"
          >
            {contact.address}
          </a>
        ) : null}
        {contact.phone ? <a href={`tel:${contact.phone}`}>{phoneText(contact.phone)}</a> : null}
        {contact.website ? (
          <a href={contact.website} target="_blank" rel="noreferrer">
            {contact.website}
          </a>
        ) : null}
      </div>
    </details>
  );
}

export default function App() {
  const [baseConfig, setBaseConfig] = useState<ConfigState | null>(null);
  const [config, setConfig] = useState<ConfigState | null>(null);
  const [configError, setConfigError] = useState<string | null>(null);
  const [selectedScenario, setSelectedScenario] = useState<string | null>(null);
  const [run, setRun] = useState<Run | null>(null);
  const [starting, setStarting] = useState(false);
  const [startError, setStartError] = useState<string | null>(null);
  const [waitStartedAt, setWaitStartedAt] = useState<number | null>(null);
  const [waitTimedOut, setWaitTimedOut] = useState(false);
  const [showMoreOpportunities, setShowMoreOpportunities] = useState(false);

  const loadBaseConfig = useCallback(async () => {
    try {
      const loaded = await fetchJson<ConfigState>("/api/config");
      setBaseConfig(loaded);
      setConfig(loaded);
    } catch (err: unknown) {
      setConfigError(err instanceof Error ? err.message : "could not load configuration");
    }
  }, []);

  const loadScenarioConfig = useCallback(async (scenarioId: string) => {
    try {
      const loaded = await fetchJson<ConfigState>(`/api/config?scenario=${encodeURIComponent(scenarioId)}`);
      setConfig({ ...loaded, demoBusinesses: baseConfig?.demoBusinesses ?? loaded.demoBusinesses });
    } catch (err: unknown) {
      setConfigError(err instanceof Error ? err.message : "could not load configuration");
    }
  }, [baseConfig?.demoBusinesses]);

  useEffect(() => {
    void loadBaseConfig();
  }, [loadBaseConfig]);

  useEffect(() => {
    if (run === null || run.status !== "running") {
      return;
    }
    const poll = () => {
      const path = run.id ? `/api/runs/${encodeURIComponent(run.id)}` : "/api/runs/latest";
      void fetchJson<Run>(path)
        .then(setRun)
        .catch(() => {
          void fetchJson<Run>("/api/runs/latest")
            .then(setRun)
            .catch(() => undefined);
        });
    };
    poll();
    const timer = setInterval(poll, 2000);
    return () => clearInterval(timer);
  }, [run?.id, run?.status]);

  useEffect(() => {
    if (run?.status !== "running") {
      setWaitStartedAt(null);
      setWaitTimedOut(false);
      return;
    }
    setWaitTimedOut(false);
    setWaitStartedAt((t) => t ?? Date.now());
  }, [run?.id, run?.status]);

  useEffect(() => {
    if (run?.status !== "running" || waitStartedAt === null) {
      return;
    }
    const left = MAX_WAIT_MS - (Date.now() - waitStartedAt);
    if (left <= 0) {
      setWaitTimedOut(true);
      return;
    }
    const timer = setTimeout(() => setWaitTimedOut(true), left);
    return () => clearTimeout(timer);
  }, [run?.status, waitStartedAt]);

  const refreshRun = () => {
    void fetchJson<Run>("/api/runs/latest")
      .then(setRun)
      .catch(() => undefined);
  };

  const pickBusiness = (scenarioId: string) => {
    setSelectedScenario(scenarioId);
    setStartError(null);
    const demos = baseConfig?.demoBusinesses ?? [];
    const picked = demos.find((b) => b.id === scenarioId);
    if (picked) {
      setConfig((prev) => ({
        ...(prev ?? baseConfig ?? { missing: [] }),
        businessName: picked.businessName,
        businessLocality: picked.locality,
        sampleProducts: picked.sampleProducts,
        demoBusinesses: demos,
      }));
    }
    void loadScenarioConfig(scenarioId);
  };

  const startAnalysis = async () => {
    setStarting(true);
    setStartError(null);
    try {
      const body =
        selectedScenario !== null ? JSON.stringify({ scenario: selectedScenario }) : undefined;
      const started = await fetchJson<{ id: string; status: string }>("/api/runs", {
        method: "POST",
        headers: body ? { "Content-Type": "application/json" } : undefined,
        body,
      });
      if (started.id) {
        setRun(await fetchJson<Run>(`/api/runs/${started.id}`));
      } else {
        setRun(await fetchJson<Run>("/api/runs/latest"));
      }
    } catch (err: unknown) {
      setStartError(err instanceof Error ? err.message : "could not start the analysis");
    } finally {
      setStarting(false);
    }
  };

  const reset = () => {
    setRun(null);
    setStartError(null);
    setShowMoreOpportunities(false);
    if ((baseConfig?.demoBusinesses?.length ?? 0) > 0) {
      setSelectedScenario(null);
      setConfig(baseConfig);
    }
  };

  const missing = baseConfig?.missing ?? config?.missing ?? [];
  const configured = baseConfig !== null && missing.length === 0;
  const showMatchChain = baseConfig?.showMatchChain === true;
  const demoBusinesses = baseConfig?.demoBusinesses ?? [];
  const showPicker = configured && run === null && demoBusinesses.length > 0 && selectedScenario === null;
  const showStart = configured && run === null && !showPicker && (demoBusinesses.length === 0 || selectedScenario !== null);
  const kicker = config ? businessKicker(config) : "";
  const name = config ? businessName(config) : "Your business";
  const productLine = config?.sampleProducts ? productWaitLine(config.sampleProducts) : null;
  const { activeIndex: waitActiveIndex, holdingResults } = useWaitStepDisplay(run);
  const showWaitScreen = run?.status === "running" || holdingResults;

  return (
    <main className="wrap">
      {configError !== null && <p className="error-box">Could not load: {configError}</p>}

      {baseConfig !== null && !configured && (
        <div className="setup-hint">
          <p className="kicker">Setup</p>
          <h1>Not ready to run yet</h1>
          <p className="lede">The server is missing: {missing.join(", ")}. Add them in the environment on the server, then reload.</p>
        </div>
      )}

      {showPicker && (
        <>
          <p className="kicker">Customer Acquisition Advisor</p>
          <h1>Select your business</h1>
          <p className="lede">
            Pick a sample business. We analyze customer sales history, call Qloo for cross-category partners, and draft
            partnership ideas for your business.
          </p>
          <div className="stack" role="list">
            {demoBusinesses.map((b) => (
              <button key={b.id} type="button" className="choice" onClick={() => pickBusiness(b.id)}>
                <strong>{b.businessName?.trim() || b.label}</strong>
                <span>{demoSubtitle(b)}</span>
              </button>
            ))}
          </div>
          <section className="production-cta" aria-label="Production path">
            <h2 className="production-cta-title">Your own business</h2>
            <p className="production-cta-lede">
              In production, connect Square for your real sales data—not enabled on this public demo.
            </p>
            <button type="button" className="btn btn-secondary" disabled title="Square connect is production-only">
              Connect Square account
            </button>
          </section>
        </>
      )}

      {showStart && (
        <>
          <p className="kicker">{kicker}</p>
          <h1>Find nearby partners</h1>
          <p className="lede">Most analyses finish in 1–2 minutes.</p>
          <div className="actions">
            <button type="button" className="btn" disabled={starting} onClick={() => void startAnalysis()}>
              Start
            </button>
            {demoBusinesses.length > 0 ? (
              <button type="button" className="btn btn-secondary" onClick={reset}>
                Choose another business
              </button>
            ) : null}
          </div>
          {startError !== null && <p className="error-box">{startError}</p>}
        </>
      )}

      {configured && showWaitScreen && (
        <div className="handoff" role="status" aria-live="polite">
          <p className="kicker">{kicker}</p>
          <h1>Working on this for you</h1>
          {productLine ? <p className="lede">{productLine}</p> : null}
          <p className="hint">Most analyses finish in 1–2 minutes.</p>
          <WaitStepList activeIndex={waitActiveIndex} />
          {waitActiveIndex === 1 ? <p className="hint">This step often takes the longest.</p> : null}
          {waitTimedOut ? (
            <div className="note">
              <h2>This is taking longer than usual</h2>
              <p className="lede">Most analyses finish in 1–2 minutes. You can check if anything came back, or start again.</p>
              <div className="actions">
                <button type="button" className="btn" onClick={refreshRun}>
                  Check for results
                </button>
                <button type="button" className="btn btn-secondary" onClick={reset}>
                  Try again
                </button>
              </div>
            </div>
          ) : null}
        </div>
      )}

      {configured && run?.status === "failed" && (
        <>
          <p className="kicker">{kicker}</p>
          <h1>We couldn&apos;t finish this for {name}</h1>
          <p className="lede">We couldn&apos;t look up nearby businesses just now. Nothing on your side was changed.</p>
          <div className="actions">
            <button type="button" className="btn" onClick={() => void startAnalysis()}>
              Try again
            </button>
            {demoBusinesses.length > 0 ? (
              <button type="button" className="btn btn-secondary" onClick={reset}>
                Choose another business
              </button>
            ) : null}
          </div>
        </>
      )}

      {configured && run?.status === "succeeded" && !holdingResults && (
        <>
          <p className="kicker">Nearby partners for {name}</p>
          <h1>Places</h1>
          <p className="lede">Each place has its own idea for working together.</p>
          {(run.opportunities ?? []).length === 0 ? (
            <p className="lede">No partnership places came back for this run.</p>
          ) : (
            <div className="places">
              {(run.opportunities ?? [])
                .slice(
                  0,
                  showMoreOpportunities
                    ? MAX_OPPORTUNITIES_VISIBLE
                    : INITIAL_OPPORTUNITIES_VISIBLE,
                )
                .map((opp) => (
                <article key={opp.candidateId ?? `${opp.placeName}-${opp.action}`} className="place">
                  <h2>{opp.placeName ?? "Place"}</h2>
                  {opp.placeType ? <p className="place-type">{opp.placeType}</p> : null}
                  {showMatchChain ? (
                    <MatchChain
                      boughtNames={run.signal?.boughtNames ?? []}
                      cohortSize={run.cohort?.length}
                      resolutions={run.resolutions ?? []}
                      placeName={opp.placeName ?? "This place"}
                      affinity={affinityForCandidate(opp.candidateId, run.qlooEvidence)}
                      crossDomainCategories={run.crossDomainCategories}
                    />
                  ) : null}
                  <p className="pitch-label">Suggested pitch</p>
                  <p className="action">{opp.action}</p>
                  <p className="why">{opp.whyFits}</p>
                  <p className="next">
                    <strong>Next. </strong>
                    {opp.nextAction}
                  </p>
                  {opp.contact ? <ContactBlock contact={opp.contact} /> : null}
                </article>
              ))}
              {(run.opportunities ?? []).length > INITIAL_OPPORTUNITIES_VISIBLE && !showMoreOpportunities ? (
                <button
                  type="button"
                  className="text-btn show-more"
                  onClick={() => setShowMoreOpportunities(true)}
                >
                  Show more
                </button>
              ) : null}
            </div>
          )}
          <button type="button" className="text-btn again" onClick={reset}>
            {demoBusinesses.length > 0 ? "Choose another business" : "Run again"}
          </button>
        </>
      )}
    </main>
  );
}
