package main

// App 结构体 (Wails 主应用)
type App = GuiApp

// NewApp 创建新应用实例
func NewApp() *GuiApp {
	return NewGuiApp()
}
