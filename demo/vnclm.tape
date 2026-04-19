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
Set TypingSpeed 30ms
Set PlaybackSpeed 1.0
Set Padding 20

Hide
Type "kubectx nova >/dev/null 2>&1; kubens vinculum-system >/dev/null 2>&1; clear"
Enter
Sleep 1500ms
Show

Type "vnclm list agents"
Sleep 200ms
Enter
Sleep 2s

Type "vnclm create agent"
Sleep 200ms
Enter
Sleep 3500ms

Type "seven-of-nine"
Sleep 800ms
Enter
Sleep 1s

Type "gpt-5.4"
Sleep 800ms
Enter
Sleep 1s

Enter
Sleep 1500ms

Tab
Sleep 800ms

Enter
Sleep 800ms

Enter
Sleep 800ms

Enter
Sleep 3500ms

Type "vnclm list agents"
Sleep 200ms
Enter
Sleep 2500ms

Type "vnclm ctx set-agent seven-of-nine"
Sleep 200ms
Enter
Sleep 1s

Type 'vnclm run --fresh --timeout 60 "Explain, in one sentence, why resistance is futile."'
Sleep 200ms
Enter
Sleep 15s

Type "vnclm list tasks"
Sleep 200ms
Enter
Sleep 3s
