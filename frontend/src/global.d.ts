// Мост окна (internal/ui/bridge.go): три метода, доступные интерфейсу через Wails.
// Call принимает имя команды и JSON запроса, отвечает JSON ответа (ipc.Reply).
interface PCBlockApp {
  Call(command: string, payload: string): Promise<string>;
  Mode(): Promise<string>;
  Version(): Promise<string>;
}

interface Window {
  go?: { ui?: { App?: PCBlockApp } };
}
