// Единая точка обращения интерфейса к программе (биндинги Wails).
// Экраны не вызывают wailsjs напрямую: так их проще тестировать с подменой.
import { Version } from "../wailsjs/go/ui/App";

export function getVersion(): Promise<string> {
  return Version();
}
