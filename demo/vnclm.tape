Output vnclm.gif

Env PATH "/Users/florianwenzel/go/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
Env KUBECONFIG "/Users/florianwenzel/.kube/config"
Env HOME "/Users/florianwenzel"
Env TERM "xterm-256color"
Env STARSHIP_CONFIG "/Users/florianwenzel/.config/starship.toml"

Set Shell "zsh"
Set FontSize 15
Set Width 1200
Set Height 1000
Set Theme "Catppuccin Mocha"
Set TypingSpeed 35ms
Set PlaybackSpeed 1.0
Set Padding 20

Hide
Type "cd ~/projects/vinculum"
Enter
Type "kubectx nova >/dev/null 2>&1; kubens vinculum-system >/dev/null 2>&1"
Enter
Type `eval "$(starship init zsh)"`
Enter
Sleep 500ms
Type "clear"
Enter
Sleep 1500ms
Show

Type "vnclm get agents"
Sleep 300ms
Enter
Sleep 2500ms

Hide
Type "clear"
Enter
Sleep 300ms
Show

Type "vnclm create agent"
Sleep 300ms
Enter
Sleep 4s

Type "seven-of-nine"
Sleep 1s
Enter
Sleep 1500ms

Type "gpt-5.4"
Sleep 1s
Enter
Sleep 1500ms

Enter
Sleep 1500ms

Enter
Sleep 1200ms

Enter
Sleep 1200ms

Enter
Sleep 1200ms

Enter
Sleep 4s

Type "vnclm get agents"
Sleep 300ms
Enter
Sleep 3s

Type 'vnclm run --agent azure-gpt4o --fresh --timeout 60 "Explain, in one sentence, why resistance is futile."'
Sleep 300ms
Enter
Sleep 16s

Type "vnclm get tasks"
Sleep 300ms
Enter
Sleep 4s
