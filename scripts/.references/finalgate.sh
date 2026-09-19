#!/bin/sh
# Final gate: brutal harness + full make test, sequentially.
cd '/media/ivintitus/Data/My Projects/Main/DevTether' || exit 1
pkill -f 'dt-brutal/bin' 2>/dev/null
sleep 0.5
sh /tmp/cline/brutal.sh > /tmp/cline/brutal3.log 2>&1
echo "BRUTAL_EXIT=$?" >> /tmp/cline/brutal3.log
make test > /tmp/cline/maketest_final.log 2>&1
echo "MAKE_EXIT=$?" >> /tmp/cline/maketest_final.log
echo GATES_DONE
