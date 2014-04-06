var app = angular.module("app", ['ngRoute', 'ngResource', 'monospaced.qrcode'], function($routeProvider){
	$routeProvider.when("/", {
		templateUrl: "/tictactoe/home.html",
		controller: "HomeCtl"
	}).when("/game/:id", {
		templateUrl: "/tictactoe/game.html",
		controller: "GameCtl"
	});
});

app.controller("MainCtl", function(){
	
});

app.controller("HomeCtl", function($http, $location){
	console.log("HOME")
	$http({
		method: "post",
		url: "/game"
	}).success(function(data) {
		$location.path("/game/" + data.uuid)
	}).error(function(err) {
		alert(err.message);
	});
});

app.controller('GameCtl', function($scope, $http, $routeParams, $http){
	$scope.id = $routeParams.id;
	$scope.state = "waiting";
	$scope.players = [];

	$scope.url = "192.168.1.106:3000"

	console.log("HERE");
	// Have to do an initial GET... workaround for martini sessions
	$http({
		method: "GET",
		url: "/game/" + $scope.id
	}).success(function(data) {
		console.log("Initial GET was successful, trying to connect via websocket.")
		$scope.isHost = data.host;
		$scope.connectWs();
	}).error(function(data, status){
		alert("Failed to get game with status " + status);
		console.log(data);
	});

	$scope.send = function(msg){}; // dummy to avoid errors?
	$scope.start = function(){
		$scope.send({type: "state", state: "start"});
	};
	$scope.move = function(space) {
		$scope.send({type: "move", move: space});
	};

	$scope.connectWs = function(){
		var conn = new WebSocket("ws://" + $scope.url + "/ws/" + $scope.id);

		conn.onclose = function(e){
			$scope.$apply(function(){
				console.log(e);
				$scope.state = "closed";
				$scope.error = e;
			});
		};

		conn.onopen = function(e){
			$scope.$apply(function(){
				console.log("CONNECTED");
			});
		};

		conn.onmessage = function(e){
			$scope.$apply(function(){
				var msg = JSON.parse(e.data);
				console.log(msg);
				switch(msg.type) {
					case "host":
						$scope.isHost = msg.host;
						break;
					case "players":
						$scope.players = msg.players;
						break;
					case "state":
						$scope.state = msg.state;
					case "update":
						$scope.state = msg.state;
						var board = [];
						for(var i=0; i<9; i++){
							if(msg.board[i] == 0){
								board.push(" ");
							} else {
								board.push("Player " + msg.board[i]);
							}
						};
						$scope.board = board;
						break;
					default:
						console.log("Unknown message type: " + msg.type);
				}
			});
		};

		$scope.send = function(msg){
			conn.send(JSON.stringify(msg));
		}
	}
});
