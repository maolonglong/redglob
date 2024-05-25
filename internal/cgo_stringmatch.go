package internal

/*
#include <ctype.h>
#include <string.h>

static int stringmatchlen_impl(const char *pattern, int patternLen,
        const char *string, int stringLen, int nocase, int *skipLongerMatches)
{
    while(patternLen) {
        switch(pattern[0]) {
        case '*':
            while (patternLen && pattern[1] == '*') {
                pattern++;
                patternLen--;
            }
            if (patternLen == 1)
                return 1;
            while(stringLen) {
                if (stringmatchlen_impl(pattern+1, patternLen-1,
                            string, stringLen, nocase, skipLongerMatches))
                    return 1;
                if (*skipLongerMatches)
                    return 0;
                string++;
                stringLen--;
            }
            *skipLongerMatches = 1;
            return 0;
            break;
        case '?':
            if (!stringLen) {
                return 0;
            }
            string++;
            stringLen--;
            break;
        case '[':
        {
            if (stringLen == 0 || patternLen < 3) {
                return 0;
            }
            int not, match;
            pattern++;
            patternLen--;
            not = pattern[0] == '^';
            if (not) {
                pattern++;
                patternLen--;
            }
            match = 0;
            while(1) {
                if (pattern[0] == '\\') {
                    if (patternLen == 1) {
                        return 0;
                    }
                    pattern++;
                    patternLen--;
                    if (pattern[0] == string[0]) {
                        if (!nocase) {
                            if (pattern[0] == string[0])
                                match = 1;
                        } else {
                            if (tolower((int)pattern[0]) == tolower((int)string[0]))
                                match = 1;
                        }
                    }
                } else if (pattern[0] == ']') {
                    break;
                } else if (patternLen == 0) {
                    return 0;
                } else if (patternLen >= 3 && pattern[1] == '-') {
                    int start = pattern[0];
                    int end = pattern[2];
                    int c = string[0];
                    if (start > end) {
                        int t = start;
                        start = end;
                        end = t;
                    }
                    if (nocase) {
                        start = tolower(start);
                        end = tolower(end);
                        c = tolower(c);
                    }
                    pattern += 2;
                    patternLen -= 2;
                    if (c >= start && c <= end)
                        match = 1;
                } else {
                    if (!nocase) {
                        if (pattern[0] == string[0])
                            match = 1;
                    } else {
                        if (tolower((int)pattern[0]) == tolower((int)string[0]))
                            match = 1;
                    }
                }
                pattern++;
                patternLen--;
            }
            if (not)
                match = !match;
            if (!match)
                return 0;
            string++;
            stringLen--;
            break;
        }
        case '\\':
            if (patternLen == 1) {
                return 0;
            }
            pattern++;
            patternLen--;
        default:
            if (!nocase) {
                if (pattern[0] != string[0])
                    return 0;
            } else {
                if (tolower((int)pattern[0]) != tolower((int)string[0]))
                    return 0;
            }
            string++;
            stringLen--;
            break;
        }
        pattern++;
        patternLen--;
        if (stringLen == 0) {
            while(*pattern == '*') {
                pattern++;
                patternLen--;
            }
            break;
        }
    }
    if (patternLen == 0 && stringLen == 0)
        return 1;
    return 0;
}

int stringmatchlen(const char *pattern, int patternLen,
        const char *string, int stringLen, int nocase) {
    int skipLongerMatches = 0;
    return stringmatchlen_impl(pattern,patternLen,string,stringLen,nocase,&skipLongerMatches);
}

int stringmatch(const char *pattern, const char *string, int nocase) {
    return stringmatchlen(pattern,strlen(pattern),string,strlen(string),nocase);
}
*/
import "C"

func OriginStringMatch(str, pattern string) bool {
	return int(C.stringmatch(C.CString(pattern), C.CString(str), C.int(0))) == 1
}
