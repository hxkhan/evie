package main

errFileNotExist := error("the requested file does not exist") // creates an error value

errFileNotExist2 := error(fn(f) => 'file {f} does not exist') // can be called to build an error value

return errFileNotExist // reciever could compared to errFileNotExist 
return error("the requested file does not exist") // will not be comparable to anything

return errFileNotExist2("file pacman.jpg does not exist") // comparable and custom message each time