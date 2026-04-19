Output vnclm.gif

Env PATH "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
Env KUBECONFIG "/Users/florianwenzel/.kube/config"
Env HOME "/Users/florianwenzel"
Env TERM "xterm-256color"

Set Shell "zsh"
Set FontSize 15
Set Width 1200
Set Height 720
Set Theme "Catppuccin Mocha"
Set TypingSpeed 45ms
Set PlaybackSpeed 1.0
Set Padding 20

Hide
Type "kubectx nova >/dev/null 2>&1; kubens vinculum-system >/dev/null 2>&1; clear"
Enter
Sleep 2s
Show

Type "vnclm list agents"
Sleep 400ms
Enter
Sleep 4s

Type "vnclm ctx set-agent azure-gpt4o"
Sleep 300ms
Enter
Sleep 2s

Type 'vnclm run --fresh --timeout 90 "write a haiku about kubernetes"'
Sleep 400ms
Enter
Sleep 28s

Type "vnclm list tasks"
Sleep 300ms
Enter
Sleep 5s
