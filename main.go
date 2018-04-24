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

func syntax() {
	fmt.Printf("Syntax: GcodeEdit [flags] GCodeFilePath\n\n")
	fmt.Printf("        -t1=190                 (sets first extruder temperature in celcius)\n")
	fmt.Printf("        -dryrun                 (no fans, no heatup, no extrusion)\n")
	fmt.Printf("        -v                      (verbose)\n")
	fmt.Printf("        -info                   (displays layer information in file only)\n")
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
	currentlayer                    := -999
	firstlayer                      := -999
	currentextrusionamount          := -999.99
	firstpositiveextrude            := -999.99
	firstgzeroline                  := ";N/A"
	linestoreplay                   := make([]string,0)
	slicer                          := "N/A"
	temp                            := "N/A"
	layers                          := "N/A"
	inputfilename                   := "N/A"
	outputfilename                  := "N/A"
	dataOut, err                    := os.Create("/tmp/GcodeEdit")
	t1                              := flag.Int("t1",        -1,    "set first extruder to indicated temp [-t1=190]")
	startfix                        := flag.Bool("startfix", false, "run first extrusion of print twice")
	dryrun                          := flag.Bool("dryrun",   false, "no heat/extrusion/fans")
	verbose                         := flag.Bool("v",        false, "verbose output")
	info                            := flag.Bool("info",     false, "just read layer data from file")
	flag.Parse()
	if len(flag.Args()) != 1        { syntax() }		// Should be only one argument as a filename after flags
  heatrelated	                    := []string{"M101", "M102", "M103", "M104", "M106", "M107",
																		 "M109", "M116", "M128", "M140", "M141", "M190", "M191"}
	/*
	dryrun-related
	M104 Set extruder temp
	M109 Set extruder temp and wait
	M140 Set bed temp
	M190 Wait for bed temp to reach target
	M141 Set chamber temp
	M191 Wait for chamber temp to reach target
	M116 Wait
	M106 Fan on
	M107 Fan off
	M101 Turn extruder 1 on
	M102 Turn extruder 1 on reverse
	M103 Turn all extruders off
	M128 Extruder pressure PWM
	*/

	inputfilename = flag.Args()[0]
	data, err := ioutil.ReadFile(inputfilename)
	if err != nil {
		bReadError = true;
		fmt.Fprintf(os.Stderr, "GcodeEdit:\n  %v\n\n", err)
		return
	}

	// When finished, this should look like...                                /Users/user/Desktop/OriginalFilename_GE.gcode
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
			if *startfix && firstlayer == currentlayer {
				if firstgzeroline == ";N/A" && strings.Contains(line, "G0 ") {
					// Haven't seen G0 yet and it's here now
					firstgzeroline = line
					linestoreplay =  append(linestoreplay, line)
					} else {
					// Either we HAVE seen G0 or we haven't reached that line yet
					if firstpositiveextrude == -999.99 && strings.Contains(line, " E") {
						// Some sort of extrusion is taking place
						currentextrusionamount, err = strconv.ParseFloat(line[strings.Index(line, " E")+2:],64)
						if currentextrusionamount > 0.0 {
							firstpositiveextrude = currentextrusionamount
							linestoreplay = append(linestoreplay, line)
							if (! *info) {
								dataOut.WriteString(line + "\n")
								dataOut.WriteString(linestoreplay[0] + "\n")
								dataOut.WriteString(linestoreplay[1] + "\n")
								dataOut.WriteString(linestoreplay[2] + "\n")
							}
						}
					}
					if firstgzeroline != ";N/A" && firstpositiveextrude == -999.99 {
						// We've seen the first G0 command but we haven't seen the first
						// positive extrusion, so add the command to the slice
						linestoreplay = append(linestoreplay, line)
					}
				}
			}
		}
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
		}
		if (! *info) {
			if line == "\n" {
				dataOut.WriteString("\n")
			} else {
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
							}
						}
					}
				}
				dataOut.WriteString(line + "\n")
			}
		}
	}
	dataOut.Sync()
	dataOut.Close()
	if !bReadError {
		fmt.Printf("Original:  %s\n", inputfilename)
		if (slicer != "N/A")                { fmt.Printf("Slicer:    %s\n", slicer) }
		if (layers != "N/A")                { fmt.Printf("Layers:    %s\n", layers) }
		if (firstlayer != -999 && !*dryrun) { fmt.Printf("First:     %d\n", firstlayer) }
		if (temp != "N/A" && !*dryrun)      { fmt.Printf("Temp:      %sC\n", temp) }
		if (! *info) {
			fmt.Printf("Editing:\n")
			fmt.Printf("  Output filename:  %s\n", outputfilename)
			if (*t1 != -1)                    { fmt.Printf("  Temp now:         %dC\n", *t1) }
			if (*dryrun)                      { fmt.Printf("  No heat/extrusion/fans\n") }
		}
		fmt.Printf("\nFinished.\n\n")
	}
}
