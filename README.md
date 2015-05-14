This lib is my own framework to build golang apps with ease.   
Modern lib has many parts and uses also many other libs that are published separately on Github.   
Main idea was to give few functions for developer to get rich functionality with few function calls.    
I suggest to not use this library in production until it will be tested more.  
Anyways, i'm sure you'll not use it until good documentation will be available :) 
So this is kind of protection..  

trivial features:

- call AppDir
- use all cores + randomize
- interrupts handler for graceful shutdown
- env variables support and parsing .env file using godotenv + envconfig
- http server with julienschmidt/httprouter and websocket support rshmelev/easyws
- supports __installservice and __phoenix options using rshmelev/librestarter and rshmelev/installasservice
- tracking mem stats + ability to fetch them via http
- logging to file(+daily rotate using modified lumberjack)
  with history for remote websocket log viewing + setting as std logger
- setting up json configs (dynamic loading with http fetching support, local json, state saving)
- http features: /healthpoint for mem stats and other info, /static file serving,
  dumping heap, smart app restart, ...

Questions?   
rshmelev@gmail.com



