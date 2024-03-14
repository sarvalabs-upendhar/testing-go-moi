package core

const (
	DOCS = "https://cocolang.dev/tour" // todo: change documentation link

	DIVIDE = "======================================================"
	STRIKE = "------------------------------------------------------"

	FIGLET = "" + DIVIDE + "\n" +
		"\u001b[32m" +
		"dP                   oo          dP          dP       \n" +
		"88                               88          88       \n" +
		"88 .d8888b. .d8888b. dP .d8888b. 88 .d8888b. 88d888b. \n" +
		"88 88'  `88 88'  `88 88 88'  `\"\" 88 88'  `88 88'  `88 \n" +
		"88 88.  .88 88.  .88 88 88.  ... 88 88.  .88 88.  .88 \n" +
		"dP `88888P' `8888P88 dP `88888P' dP `88888P8 88Y8888' \n" +
		"                 .88                                  \n" +
		"             d8888P                                   \n" +
		"\u001b[0m" +
		DIVIDE + "\n"

	CLOSER = "\r" + DIVIDE + "\nClosing LogicLab REPL"
	LAUNCH = FIGLET +
		"LogicLab Initialized @ \u001B[32m%v\u001B[0m\n" +
		"LogicLab Documentation: %v\n" +
		"\u001B[32mStarting LogicLab in %v Mode...\u001B[0m (use 'ctrl-c' to stop)\n" +
		DIVIDE
)
