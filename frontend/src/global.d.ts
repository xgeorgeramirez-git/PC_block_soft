// Мост окна (internal/ui/app.go): пять методов, доступные интерфейсу через Wails (Ruling 14).
// Call принимает имя команды и JSON запроса, отвечает JSON ответа (ipc.Reply).
// Finish закрывает окно с кодом выхода; в подмене и старых тестах его может не быть.
interface PCBlockApp {
  Call(command: string, payload: string): Promise<string>;
  Mode(): Promise<string>;
  /** Экран, на котором открыть окно родителя (RM3-4); в подмене и старых тестах метода может не быть. */
  StartScreen?(): Promise<string>;
  Version(): Promise<string>;
  Finish?(code: number): Promise<void>;
}

interface Window {
  go?: { ui?: { App?: PCBlockApp } };
}
