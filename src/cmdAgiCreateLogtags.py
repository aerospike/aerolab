#!/usr/bin/env python3

from __future__ import print_function
import sys
import argparse
#import re
#import os
#import os.path

# Input sample and fields:
#   0  1    2        3         4       5     6           7 8 ...
# Oct 31 2019 03:26:11 GMT-0700: WARNING (hb): (hb.c:4869) heartbeat TLS client handshake with {10.110.28.48:3012} failed

def print_debug(debug, *args):
  if debug:
    print(' '.join(['DEBUG:'] + args))

def parse_arguments(args):
  # FIXME TODO make these actually do something
  # -r  --clock-time         show first and last lines according to time
  parser = argparse.ArgumentParser(description='logtags.py 4 lyfe')

  # True/False args
  parser.add_argument('-H', '--hist', action='store_true', 
    help='show "hist.c" tags instead of omitting them')
  parser.add_argument('-t', '--ticker', action='store_true', 
    help='show "ticker.c" tags instead of omitting them')
  parser.add_argument('-v', '--verbose', action='store_true', 
    help='show first and last instances of each tag seen in input')
  #parser.add_argument('-r', '--clock-time', action='store_true', help='NOT IMPLEMENTED')
  parser.add_argument('-D', '--debug', action='store_true', 
    help='spew a bunch of useless debug information')
  parser.add_argument('-a', '--all', '--aggregate', action='store_true', 
    help='treat all input as a single logfile, instead of processing each one separately')

  # Args with values
  parser.add_argument('-l', '--log-level', '--loglevel', nargs=1, action='store', default='warning', 
    help='spammiest log level to include (CRITICAL and FAILED_ASSERTION are synonyms), not case-sensitive, unique prefixes okay, default WARNING')
  parser.add_argument('-c', '--context', nargs=1, action='append', 
    help='include only tags with these contexts in output (includes all if not set)')
  parser.add_argument('-C', '--exclude-context', nargs=1, action='append', 
    help='exclude tags with these contexts from output')

  # Any non-option arguments are files to read
  parser.add_argument('inputs', nargs='*', action='store', 
    help='files to read (stdin if not set)')

  options = vars(parser.parse_args(args))
  DEBUG = options['debug']
  print_debug(DEBUG, 'raw options:', str(options))

  # Flatten context and exclude-context list-of-lists
  # I'm not too proud to use stackexchange: flat_list = [item for sublist in l for item in sublist]
  options['context_include_list'] = []
  if options['context']:
    options['context_include_list'] = [item for sublist in options['context'] for item in sublist]
  options['context_exclude_list'] = []
  if options['exclude_context']:
    options['context_exclude_list'] = [item for sublist in options['exclude_context'] for item in sublist]

  # Add () to contexts here so we don't have to worry when matching input lines
  def parenthesize(cont):
    return '(' + cont.strip('():') + ')'
  options['context_include_list'] = [parenthesize(a) for a in options['context_include_list']]
  options['context_exclude_list'] = [parenthesize(a) for a in options['context_exclude_list']]

  # The value provided by the user might only be a prefix, so we
  # need to case-insensitively match it against the possibilities to
  # see which one it really is (and handle failures if it doesn't
  # match exactly one). Then we need to make a set to efficiently
  # check incoming log lines against.
  lll = ['DETAIL', 'DEBUG', 'INFO', 'WARNING', 'CRITICAL', 'FAILED_ASSERTION']
  options['log_level'] = options['log_level'][0].upper()
  print_debug(DEBUG,'log_level', options['log_level']);
  pll = []
  for ii in lll:
    if ii.startswith(options['log_level']):
      pll.append(ii)
  
  if len(pll) == 0:
    print('ERROR Unknown log level', options['log_level'])
    sys.exit(2)
  elif len(pll) > 1:
    print('ERROR Ambiguous log level', str(pll))
    sys.exit(2)
  else:
    options['log_level'] = pll[0]
    if options['log_level'] == 'FAILED_ASSERTION':
      options['log_level'] = 'CRITICAL'

  # Truncate everything earlier in the list than our value, and
  # make it into a set.
  ii = lll.index(options['log_level'])
  options['log_filter_set'] = set(lll[ii:])

  print_debug(DEBUG, 'cooked options:', str(options))

  return options



def consolidate_input(input, options, records_by_tag):
  DEBUG = options['debug']

  # Example record
  #            0      1         2        3               4                 5               6           7
  # Dict of [tag, count, loglevel, context, example message, first full line, last full line, input name]

  for line in input[0]:
    # Change the two-word loglevel "FAILED ASSERTION" into one word so split() works right.
    line = line.replace('FAILED ASSERTION', 'FAILED_ASSERTION', 1).strip()
    fields = line.split()
    # Sometimes we get other lines in "log output", which I blame
    # on systemd. Try to filter these without wasting too much time
    # in our loop.
    if len(fields) < 8:
      continue
    # Implement the -l option
    if fields[5] not in options['log_filter_set']:
      continue
    tag = fields[7]
    if tag in records_by_tag:
      # Only increment count and last-seen whole line
      records_by_tag[tag][1] = records_by_tag[tag][1] + 1 
      records_by_tag[tag][6] = line
      #print('DEV: updated', tag, 'with', line)
    else:
      # Make an entire new entry with a count of 1
      records_by_tag[tag] = [tag, 1, fields[5], fields[6].strip(':'), ' '.join(fields[8:]), line, line, input[1]]

  return records_by_tag



def sort_filter_records(records_by_tag, options):
  DEBUG = options['debug']
  # After this point, we will use records, a list, instead of records_by_tag, a dict.
  records = records_by_tag.values()

  # Example record and fields
  #   0      1         2        3               4                 5               6           7
  # [tag, count, loglevel, context, example message, first full line, last full line, input name]

  sorted_records = sorted(records, key=lambda a: int(a[1]))
  print_debug(DEBUG, 'sorted tags/contexts:', str([(rec[0], rec[3]) for rec in sorted_records]))

  # Implement the -c and -C options
  filtered_records = sorted_records
  if options['context_include_list']:
    filtered_records = [rec for rec in filtered_records if rec[3] in options['context_include_list']]
    print_debug(DEBUG, 'after considering context_include_list:', [(ii[0], ii[3]) for ii in filtered_records])
  if options['context_exclude_list']:
    filtered_records = [rec for rec in filtered_records if rec[3] not in options['context_exclude_list']]
    print_debug(DEBUG, 'after considering context_exclude_list:', [(ii[0], ii[3]) for ii in filtered_records])

  # Implement -H and -t options
  if not options['hist']:
    filtered_records = [rec for rec in filtered_records if not rec[0].startswith('(hist.c:')]
  if not options['ticker']:
    filtered_records = [rec for rec in filtered_records if not rec[0].startswith('(ticker.c:')]

  print_debug(DEBUG, 'filtered tags/contexts:', str([(rec[0], rec[3]) for rec in filtered_records]))

  return filtered_records

def print_records(records, options):
  # FIXME - make a format for neatly columnated output

  for record in records:
    print(record[1], record[2], record[0], record[3], 'EX:', record[4])
    if options['verbose']:
      print('    ', record[5])
      print('    ', record[6])



def main(argv):
  progname = argv[0]
  args = argv[1:]

  options = parse_arguments(args)
  DEBUG = options['debug']

  if len(options['inputs']) == 0:
    if sys.stdin.isatty() :
      print('ERROR: stdin should be a pipe, not tty')
      sys.exit(3)
    else:
      valid_inputs = [(sys.stdin, '(stdin)')]
  else:
    valid_inputs = []
    for ii in options['inputs']:
      try:
        ff = open(ii, 'r')
        valid_inputs.append((ff, str(ii)))
      except Exception as e:
        print('WARNING: could not open', ii, 'to read:', e)
        continue

  if len(valid_inputs) == 0:
    print('ERROR: no input files could be read')
    sys.exit(3)

  records_by_tag = {}

  for input in valid_inputs:
    if len(valid_inputs) > 1:
      print('==> ', input[1], ' <==')
    records_by_tag = consolidate_input(input, options, records_by_tag)
    print_debug(DEBUG, 'tags:', records_by_tag.keys())
    # If we're not combining all the inputs, produce output after
    # each input source is finished, and then start fresh for the
    # next one.
    if not options['all']:
      records = sort_filter_records(records_by_tag, options)
      print_records(records, options)
      records_by_tag = {}

  # But if we are combining all the inputs, produce the output at
  # the end.
  if options['all']:
    records = sort_filter_records(records_by_tag, options)
    print_records(records, options)
  

if __name__ == '__main__':
  main(sys.argv)