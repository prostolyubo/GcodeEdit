/*
GcodeEdit

Description: An executable to modify the indicated GCODE file, suitable for
						 post-slice modifications.

Notes:       The current version expects Cura as the slicer and has been tested
						 with v2.3.1 of same. It has been tested with the Robo C2 printer
						 with a single extruder and no heated bed.

Author:      Michael Blankenship
Repo:        https://github.com/OutsourcedGuru/GcodeEdit
*/
package main
import (
	"flag"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"os"
	"regexp"
	"strconv"
	"strings"
)

func showversion() {
	fmt.Printf("v1.0.4\n")
	os.Exit(0)
}

func syntax(more string) {
	fmt.Printf("Syntax: GcodeEdit [flags] GCodeFilePath\n\n")
	fmt.Printf("        -t1=190                 (sets first extruder temperature in celcius)\n")
	fmt.Printf("        -dryrun                 (no fans, no heatup, no extrusion)\n")
	fmt.Printf("        -verbose                (verbose)\n")
	fmt.Printf("        -info                   (displays layer information in file only)\n")
	fmt.Printf("        --version | -v          (displays version information)\n")
  if (more != "") {
		fmt.Printf("\nError: %s\n", more)
	}
	fmt.Printf("\n")
	os.Exit(1)
}

func commandInList(a string, list []string) bool {
	for _, b := range list {if b == a {return true}}
	return false
}

func main() {
	bReadError                      := false
	bHeader                         := true
	bNotReplayedYet                 := true
	currentlayer                    := -999
	firstlayer                      := -999
	slicer                          := "N/A"
	temp                            := "N/A"
	layers                          := "N/A"
	inputfilename                   := "N/A"
	outputfilename                  := "N/A"
	firstironline                   := ";N/A"
	saveline                        := ";N/A"
	optioncount                     := 0
	ironlayer                       := 999
	linestoreplay                   := make([]string,0)
	dataOut, err                    := os.Create("/tmp/GcodeEdit")
	t1                              := flag.Int("t1",        -1,    "set first extruder to indicated temp [-t1=190]")
	iron                            := flag.Int("iron",      -99,   "iron the indicated layer [-iron=3]")
	dryrun                          := flag.Bool("dryrun",   false, "no heat/extrusion/fans")
	verbose                         := flag.Bool("verbose",  false, "verbose output")
	info                            := flag.Bool("info",     false, "just read layer data from file")
	v                               := flag.Bool("v",        false, "show version information")
	version                         := flag.Bool("version",  false, "show version information")
	heatrelated	                    := []string{"M101", "M102", "M103", "M104", "M106", "M107",
		                              "M109", "M116", "M128", "M140", "M141", "M190", "M191"}
	flag.Parse()
	if (*v && (*dryrun || *iron != -99))                 { syntax("Use -verbose with -dryrun or -iron; -v shows version info") }
  if (*v || *version)                                  { showversion() }
	if len(flag.Args()) != 1                             { syntax("") }		// Should be only one argument as a filename after flags
	if (!*dryrun && *t1 == -1 && !*info && *iron == -99) { syntax("Minimally, add a command flag to perform an operation on the file") }
	if (*verbose && ! (*dryrun || *iron != -99))         { syntax("Use -verbose with -dryrun or -iron") }
	if (*iron <= 1 && *iron > -99)                       { syntax("Minimum -iron value should be 2") }
	if (*dryrun) {optioncount++}
	if (*t1 != -1) {optioncount++}
	if (*iron != -99) {optioncount++}
	if optioncount > 1                                   { syntax("Only one of: -dryrun, -t1 or -iron flags at a time") }
  if (*iron != -99)                                    { ironlayer = *iron - 1 }
	/*
	dryrun-related
	M104 Set extruder temp         	  M109 Set extruder temp and wait
	M140 Set bed temp             	  M190 Wait for bed temp to reach target
	M141 Set chamber temp         	  M191 Wait for chamber temp to reach target
	M116 Wait                     	  M106 Fan on
	M107 Fan off                  	  M101 Turn extruder 1 on
	M102 Turn extruder 1 on reverse	  M103 Turn all extruders off
	M128 Extruder pressure PWM
	*/

	// Open the indicated file
	inputfilename = flag.Args()[0]
	data, err := ioutil.ReadFile(inputfilename)
	if err != nil {
		bReadError = true;
		fmt.Fprintf(os.Stderr, "GcodeEdit:\n  %v\n\n", err)
		return
	}

	// Output filename should look like...                                    /Users/user/Desktop/OriginalFilename_GE.gcode
	path, basefilename := filepath.Split(inputfilename)                       // OriginalFilename.gcode
	basefilename =   basefilename[0:strings.LastIndexAny(basefilename, ".")]	// OriginalFilename
	outputfilename = fmt.Sprintf("%s%s_GE%s", path, basefilename, filepath.Ext(inputfilename))
	// Attempt to create the output file
	if (! *info) {
		dataOut, err = os.Create(outputfilename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "GcodeEdit:\n  %v\n\n", err)
			return
		}
	}

	// Now process the input file into the output file
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, ";LAYER:")   {
			currentlayer, err = strconv.Atoi(line[7:])
			if firstlayer == -999 {
				firstlayer = currentlayer
			}
		} else {
		// ----------------------------------------------------------------------
		// Processing for iron command
		// ----------------------------------------------------------------------
		if *iron != -1 && ironlayer == currentlayer {
				if firstironline == ";N/A" {
					// Haven't seen the to-be-ironed layer before but we're there now
					firstironline = line
					s := fmt.Sprintf(";LAYER:%d replay per iron-processing command: GcodeEdit)", currentlayer)
					linestoreplay = append(linestoreplay, s)
					if (line[:1] != "M") {
						linestoreplay = append(linestoreplay, line)
					}
				} else {
					if (strings.Contains(line, " E") && (line[:2] == "G0" || line[:2] == "G1")) {
						// Some sort of extrusion is taking place; must remove that part
						if (*verbose) {fmt.Printf("  Before:    %s\n", line)}
						re := regexp.MustCompile(" E([0-9\\.]+)")
						saveline = re.ReplaceAllString(line, "");
						if (*verbose) {fmt.Printf("  After:     %s\n", line)}
						orphanedFCommand, err := regexp.MatchString("G[0-1] F([0-9]+)[ ]?$", saveline)
						if (err == nil && orphanedFCommand) {
							// In some cases, this then leaves nothing but G1 F2400 left after
							// the removal of the extrusion command; let's lose that, too.
							saveline = ";" + saveline
							if (*verbose) {fmt.Printf("  Commented: %s\n", saveline)}
						} // if (err == nil
						linestoreplay = append(linestoreplay, saveline)
					} else if (line[:1] == ";") {
						linestoreplay = append(linestoreplay, line)
					}   // else if (strings.Contains(line, " E")
				}     // else if firstironline == ";N/A"
			}       // if *iron != -1
 		}         // else if strings.Contains(line, ";LAYER:")
		// ----------------------------------------------------------------------
		// End of iron processing, see below as well
		// ----------------------------------------------------------------------

		// ----------------------------------------------------------------------
		// Header processing (info and t1)
		// ----------------------------------------------------------------------
		if bHeader {
			// M104 S190
			if strings.Contains(line, "M104") || strings.Contains(line, "M109") {
				temp = line[6:]
				if (*t1 != -1) {
					if strings.Contains(line, "M104") { line = fmt.Sprintf("M104 S%d", *t1)}
					if strings.Contains(line, "M109") { line = fmt.Sprintf("M109 S%d", *t1)}
				}
			}
			if strings.Contains(line, ";Generated with") { slicer = line[16:] }
			// This represents the last line of the Cura header
			if strings.Contains(line, ";LAYER_COUNT:")   {
				bHeader = false
				layers = line[13:]
			}
		} // end of if bHeader
		// ----------------------------------------------------------------------
		// End of header processing
		// ----------------------------------------------------------------------
		if (! *info) {
			if line == "\n" {
				dataOut.WriteString("\n")
			} else {
				// ------------------------------------------------------------------
				// dryrun processing
				// ------------------------------------------------------------------
				if (*dryrun && len(line) > 1) {
					if (commandInList(line[:4], heatrelated)) {
						// dryrun action: comment-out M104 and similar heat-related commands
						line = ";" + line;
						if (*verbose) {fmt.Printf("  Commented: %s\n", line)}
					} else {
						if (strings.Contains(line, " E") && (line[:2] == "G0" || line[:2] == "G1")) {
							if (*verbose) {fmt.Printf("  Before:    %s\n", line)}
							re := regexp.MustCompile(" E([0-9\\.]+)")
							line = re.ReplaceAllString(line, "");
							if (*verbose) {fmt.Printf("  After:     %s\n", line)}
							orphanedFCommand, err := regexp.MatchString("G[0-1] F([0-9]+)[ ]?$", line)
							if (err == nil && orphanedFCommand) {
								// In some cases, this then leaves nothing but G1 F2400 left after
								// the removal of the extrusion command; let's lose that, too.
								line = ";" + line
								if (*verbose) {fmt.Printf("  Commented: %s\n", line)}
							} // if (err == nil
						}   // if (strings.Contains(line, " E")
					}     // if (commandInList(
				}       // if (*dryrun && len(line) > 1)
				// ------------------------------------------------------------------
				// end of dryrun processing
				// ------------------------------------------------------------------

				// ------------------------------------------------------------------
				// Pre-processing for replay of iron
				// ------------------------------------------------------------------
				if *iron != -1 && (ironlayer + 1) == currentlayer && bNotReplayedYet {
					for _, val := range linestoreplay {
						dataOut.WriteString(val + "\n")
					}     // for _, val
					dataOut.WriteString(";End of iron-processing replay: GcodeEdit\n")
					bNotReplayedYet = false
				}       // if *iron != -1
				// ------------------------------------------------------------------
				// End of iron processing
				// ------------------------------------------------------------------

				// line has been processed, now write it to the file
				dataOut.WriteString(line + "\n")

			}         // else for if line == "\n"
		}           // if ! *info
	}             // for loop
	dataOut.Sync()
	dataOut.Close()
	if !bReadError {
		fmt.Printf("Original:  %s\n", inputfilename)
		if (slicer != "N/A")                { fmt.Printf("Slicer:    %s\n", slicer) }
		if (layers != "N/A")                { fmt.Printf("Layers:    %s\n", layers) }
		if (temp != "N/A" && !*dryrun)      { fmt.Printf("Temp:      %sC\n", temp) }
		if (! *info) {
			fmt.Printf("Editing:\n")
			fmt.Printf("  Output filename:  %s\n", outputfilename)
			if (*t1 != -1)                    { fmt.Printf("  Temp now:         %dC\n", *t1) }
			if (*dryrun)                      { fmt.Printf("  No heat/extrusion/fans\n") }
			if (*iron != -99) {
				fmt.Printf("  Replayed lines:   %d\n", len(linestoreplay))
				if (*verbose) {
					for k, val := range linestoreplay {
						fmt.Printf("  Replay[%d]: %s\n", k, val)
					}
				}
			}
		}
		fmt.Printf("\nFinished.\n\n")
	}
}
