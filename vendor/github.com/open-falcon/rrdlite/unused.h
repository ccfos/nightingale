/* define a macro to wrap variables that would
   otherwise generate UNUSED variable warnings
   Note that GCC's attribute unused only supresses the warning, so
   it is perfectly safe to declare something unused although it is not.
*/

#ifdef UNUSED
#elif defined(__GNUC__)
# define UNUSED(x) x __attribute__((unused))
#elif defined(__LCLINT__)
# define UNUSED(x) /*@unused@*/ x
#else
# define UNUSED(x) x
#endif
