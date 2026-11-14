// Обёртка проигрывателя стикеров lottie-web 5.13.0 (RM5b-8, уточнение 13 плана M5b).
// Файл vendor/lottie_light.min.js собран как UMD: без module/define он кладёт проигрыватель
// в window.lottie. В сборку он идёт отдельным файлом (?url) и грузится тегом <script> только
// когда нужен стикер, а не вместе со всем окном. В vitest эту обёртку подменяют.
import playerUrl from "../vendor/lottie_light.min.js?url";

export interface AnimationItem {
  readonly totalFrames: number;
  play(): void;
  stop(): void;
  goToAndStop(value: number, isFrame?: boolean): void;
  addEventListener(name: "complete" | "DOMLoaded" | "data_failed", cb: () => void): void;
  destroy(): void;
}

export interface LoadAnimationParams {
  container: Element;
  renderer: "svg";
  loop?: boolean;
  autoplay?: boolean;
  animationData: unknown;
  rendererSettings?: { preserveAspectRatio?: string; progressiveLoad?: boolean };
}

export interface LottiePlayer {
  loadAnimation(params: LoadAnimationParams): AnimationItem;
}

type WithLottie = { lottie?: LottiePlayer };

let pending: Promise<LottiePlayer> | null = null;

/** Загружает проигрыватель один раз; при сбое следующий вызов пробует снова. */
export function loadLottie(url: string = playerUrl): Promise<LottiePlayer> {
  const ready = (window as WithLottie).lottie;
  if (ready) return Promise.resolve(ready);
  if (pending) return pending;
  const p = new Promise<LottiePlayer>((resolve, reject) => {
    const s = document.createElement("script");
    s.src = url;
    s.async = true;
    s.dataset.lottie = "1";
    s.addEventListener("load", () => {
      const player = (window as WithLottie).lottie;
      if (player) resolve(player);
      else fail(new Error(`lottie: нет window.lottie после ${url}`));
    });
    s.addEventListener("error", () => fail(new Error(`lottie: не загрузился ${url}`)));
    function fail(e: Error) {
      s.remove();
      if (pending === p) pending = null;
      reject(e);
    }
    document.head.appendChild(s);
  });
  pending = p;
  return p;
}

/** Только для тестов: забыть загрузку. */
export function resetLottieForTest(): void {
  pending = null;
}
