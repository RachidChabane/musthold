# The agent loop of the committed recording, as a GIF: `demo/loop/run gif OUT`.
# vhs renders it; every line shown comes from demo/loop/recording/transcript.md.

Set Shell "bash"
Set FontSize 15
Set Width 1180
Set Height 720
Set Padding 24
Set Theme "GitHub Dark"
Set TypingSpeed 0

Hide
Type "clear && python3 demo/loop/gif/scenes.py --pause 2.6 demo/loop/recording/transcript.md"
Enter
Show
Sleep 22s
