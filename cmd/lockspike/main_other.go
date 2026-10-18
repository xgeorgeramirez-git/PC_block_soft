//go:build !windows

// Проба экрана блокировки существует только для Windows; на других ОС команда
// просто сообщает об этом (чтобы go vet ./... и go build ./... не падали).
package main

import "fmt"

func main() {
	fmt.Println("lockspike: проба доступна только в Windows")
}
